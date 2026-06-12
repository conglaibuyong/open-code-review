package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/open-code-review/open-code-review/internal/agent"
	"github.com/open-code-review/open-code-review/internal/config/rules"
	"github.com/open-code-review/open-code-review/internal/config/template"
	"github.com/open-code-review/open-code-review/internal/config/toolsconfig"
	"github.com/open-code-review/open-code-review/internal/diff"
	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/stdout"
	"github.com/open-code-review/open-code-review/internal/telemetry"
	"github.com/open-code-review/open-code-review/internal/tool"
	"github.com/open-code-review/open-code-review/internal/vcs"
)

func runReview(args []string) error {
	opts, err := parseReviewFlags(args)
	if err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if opts.showHelp {
		printReviewUsage()
		return nil
	}

	// Detect or validate VCS backend
	vcsBackend, err := resolveVCSBackend(opts)
	if err != nil {
		return err
	}

	// Create VCS provider
	vcsProv, err := createVCSProvider(vcsBackend, opts)
	if err != nil {
		return err
	}

	// Validate repository
	repoDir, err := vcsProv.ResolveRepoDir(opts.repoDir)
	if err != nil {
		return fmt.Errorf("resolve repo: %w", err)
	}

	tpl, err := template.LoadDefault()
	if err != nil {
		return fmt.Errorf("load default template: %w", err)
	}
	if opts.maxTools > 0 {
		tpl.MaxToolRequestTimes = opts.maxTools
	}
	if err := tpl.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	if opts.commit != "" && opts.background == "" {
		if msg, err := vcsProv.GetCommitMessage(repoDir, opts.commit); err == nil && msg != "" {
			opts.background = msg
		}
	}

	resolver, fileFilter, err := rules.NewResolver(repoDir, opts.rulePath)
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}

	if opts.preview {
		return runPreview(repoDir, opts, fileFilter, vcsProv)
	}

	toolEntries, err := toolsconfig.Load(opts.toolConfigPath)
	if err != nil {
		return fmt.Errorf("load tools: %w", err)
	}
	planToolDefs := agent.BuildToolDefs(toolEntries, true)
	mainToolDefs := agent.BuildToolDefs(toolEntries, false)

	cfgPath, err := defaultConfigPath()
	if err != nil {
		return err
	}

	appCfg, err := LoadAppConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("load app config: %w", err)
	}
	if appCfg != nil {
		tpl.ApplyLanguage(appCfg.Language)
	}

	ep, err := llm.ResolveEndpoint(cfgPath)
	if err != nil {
		return fmt.Errorf("resolve LLM endpoint: %w", err)
	}

	llmClient := llm.NewLLMClient(ep)
	model := ep.Model

	collector := tool.NewCommentCollector()
	mode := tool.ParseReviewMode(opts.from, opts.to, opts.commit, opts.shelveset)
	ref, _ := mode.RefValue(opts.to, opts.commit)
	fileReader := &tool.FileReader{
		RepoDir: repoDir,
		Mode:    mode,
		Ref:     ref,
		VCSProv: vcsProv,
	}
	// Set Runner for backward compatibility with code_search.go (git grep)
	if gitProv, ok := vcsProv.(*vcs.GitProvider); ok {
		fileReader.Runner = gitProv.Runner()
	}
	tools := buildToolRegistry(collector, fileReader)

	ag := agent.New(agent.Args{
		RepoDir:               repoDir,
		From:                  opts.from,
		To:                    opts.to,
		Commit:                opts.commit,
		Shelveset:             opts.shelveset,
		Template:              *tpl,
		SystemRule:            resolver,
		FileFilter:            fileFilter,
		LLMClient:             llmClient,
		Tools:                 tools,
		PlanToolDefs:          planToolDefs,
		MainToolDefs:          mainToolDefs,
		CommentCollector:      collector,
		CommentWorkerPool:     agent.NewCommentWorkerPool(opts.concurrency),
		MaxConcurrency:        opts.concurrency,
		ConcurrentTaskTimeout: opts.perFileTimeout,
		Model:                 model,
		Background:            opts.background,
		VCSProvider:           vcsProv,
	})

	// Silence progress output during execution; restore before Summary in agent mode.
	var unsilence func()
	if opts.outputFormat == "json" || opts.audience == "agent" {
		unsilence = stdout.Quiet()
		defer func() {
			if unsilence != nil {
				unsilence()
			}
		}()
	}

	ctx, span := telemetry.StartSpan(context.Background(), "review.run")
	defer span.End()
	startTime := time.Now()

	comments, err := ag.Run(ctx)
	if err != nil {
		telemetry.SetAttr(span, "error", err.Error())
		return fmt.Errorf("review failed: %w", err)
	}

	// Resolve line numbers by matching existing_code against diff hunks.
	comments = diff.ResolveLineNumbers(comments, ag.Diffs())

	// Record summary metrics (files_reviewed is refined by agent.Run).
	duration := time.Since(startTime)
	telemetry.RecordReviewDuration(ctx, duration)
	if len(comments) > 0 {
		telemetry.RecordCommentsGenerated(ctx, int64(len(comments)))
	}

	// If no files were reviewed (e.g. workspace has no changes), inform the caller in JSON mode.
	if opts.outputFormat == "json" && len(comments) == 0 && ag.FilesReviewed() == 0 {
		return outputJSONNoFiles()
	}

	// In agent mode (text output), restore stdout so Summary reaches the terminal.
	if opts.audience == "agent" && opts.outputFormat != "json" && unsilence != nil {
		unsilence()
		unsilence = nil
	}

	if opts.outputFormat != "json" {
		telemetry.PrintTraceSummary(ag.FilesReviewed(), int64(len(comments)), ag.TotalInputTokens(), ag.TotalOutputTokens(), ag.TotalTokensUsed(), ag.TotalCacheReadTokens(), ag.TotalCacheWriteTokens(), duration)
	}

	if opts.outputFormat == "json" {
		return outputJSONWithWarnings(comments, ag.Warnings(), ag.FilesReviewed(), ag.TotalInputTokens(), ag.TotalOutputTokens(), ag.TotalTokensUsed(), ag.TotalCacheReadTokens(), ag.TotalCacheWriteTokens(), duration)
	}
	if opts.audience == "agent" {
		outputTextWithWarnings(comments, ag.Warnings())
		return nil
	}
	outputTextWithWarnings(comments, ag.Warnings())

	return nil
}

// resolveVCSBackend determines which VCS backend to use based on the --vcs flag
// and auto-detection.
func resolveVCSBackend(opts reviewOptions) (vcs.Backend, error) {
	switch opts.vcs {
	case "git":
		return vcs.Git, nil
	case "tfvc":
		return vcs.TFVC, nil
	case "auto":
		dir := opts.repoDir
		if dir == "" {
			dir, _ = os.Getwd()
		}
		backend, err := vcs.DetectBackend(dir)
		if err != nil {
			return "", fmt.Errorf("cannot detect VCS: %w\nUse --vcs git or --vcs tfvc to specify", err)
		}
		return backend, nil
	default:
		return "", fmt.Errorf("invalid --vcs value %q: must be 'auto', 'git', or 'tfvc'", opts.vcs)
	}
}

// createVCSProvider creates the appropriate VCS provider for the given backend.
func createVCSProvider(backend vcs.Backend, opts reviewOptions) (vcs.Provider, error) {
	switch backend {
	case vcs.Git:
		return vcs.NewGitProvider(opts.maxGitProcs), nil
	case vcs.TFVC:
		return vcs.NewTFVCProvider(""), nil
	default:
		return nil, fmt.Errorf("unsupported VCS backend: %s", backend)
	}
}

// requireVCSRepo validates that the given directory is a valid VCS repository.
// Deprecated: Use vcsProvider.ResolveRepoDir instead.
func requireVCSRepo(dir string, vcsProv vcs.Provider) error {
	repoDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	if !vcsProv.Detect(repoDir) {
		return fmt.Errorf("%s is not a valid repository, code review requires a valid repository", repoDir)
	}
	return nil
}

func runPreview(repoDir string, opts reviewOptions, fileFilter *rules.FileFilter, vcsProv vcs.Provider) error {
	ag := agent.New(agent.Args{
		RepoDir:     repoDir,
		From:        opts.from,
		To:          opts.to,
		Commit:      opts.commit,
		Shelveset:   opts.shelveset,
		FileFilter:  fileFilter,
		VCSProvider: vcsProv,
	})

	preview, err := ag.Preview(context.Background())
	if err != nil {
		return fmt.Errorf("preview failed: %w", err)
	}

	outputPreviewText(preview)
	return nil
}

func buildToolRegistry(collector *tool.CommentCollector, fr *tool.FileReader) *tool.Registry {
	reg := tool.NewRegistry()
	reg.Register(tool.NewFileRead(fr))
	reg.Register(tool.NewFileFind(fr))
	reg.Register(tool.NewFileReadDiff(tool.DiffMap{}))
	reg.Register(tool.NewCodeSearch(fr))
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	return reg
}
