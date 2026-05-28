import React, { useState } from 'react';
import { useTranslation } from '../i18n';

interface LeaderboardEntry {
  rank: number;
  medal?: string;
  model: string;
  provider: string;
  source: string;
  sourceType: 'ocr' | 'claude';
  version: string;
  f1: string;
  precision: string;
  precisionFraction: string;
  recall: string;
  recallFraction: string;
  tags: string[];
}

const leaderboardData: LeaderboardEntry[] = [
  {
    rank: 1,
    medal: 'gold',
    model: 'Claude-4.6-Opus',
    provider: 'Anthropic',
    source: 'Open Code Review',
    sourceType: 'ocr',
    version: 'AVG (10)',
    f1: '26.1%',
    precision: '41.9%',
    precisionFraction: '286/682',
    recall: '19.0%',
    recallFraction: '286/1505',
    tags: ['Closed', 'Reproduced']
  },
  {
    rank: 2,
    medal: 'silver',
    model: 'Qwen-3.6-Plus',
    provider: 'Alibaba',
    source: 'Open Code Review',
    sourceType: 'ocr',
    version: 'AVG (3)',
    f1: '21.9%',
    precision: '38.6%',
    precisionFraction: '231/597',
    recall: '15.3%',
    recallFraction: '231/1505',
    tags: ['Reproduced', 'Closed']
  },
  {
    rank: 3,
    medal: 'bronze',
    model: 'GLM-4.7',
    provider: 'Zhipu AI',
    source: 'Open Code Review',
    sourceType: 'ocr',
    version: 'AVG (2)',
    f1: '20.1%',
    precision: '34.2%',
    precisionFraction: '214/624',
    recall: '14.2%',
    recallFraction: '214/1505',
    tags: ['Open Source', 'Reproduced']
  },
  {
    rank: 4,
    medal: undefined,
    model: 'Qwen3.5-27b-dense',
    provider: 'Alibaba',
    source: 'Open Code Review',
    sourceType: 'ocr',
    version: 'AVG (3)',
    f1: '19.5%',
    precision: '28.1%',
    precisionFraction: '225/799',
    recall: '15.0%',
    recallFraction: '225/1505',
    tags: ['Open Source']
  },
  {
    rank: 5,
    medal: undefined,
    model: 'Claude-4.5-Sonnet',
    provider: 'Anthropic',
    source: 'Claude Code + Skills',
    sourceType: 'claude',
    version: 'v0',
    f1: '15.5%',
    precision: '30.0%',
    precisionFraction: '157/523',
    recall: '10.4%',
    recallFraction: '157/1505',
    tags: ['Closed', 'Reproduced']
  },
  {
    rank: 6,
    medal: undefined,
    model: 'Qwen3-Coder-480B-A35B-Instruct',
    provider: 'Alibaba',
    source: 'Claude Code + Skills',
    sourceType: 'claude',
    version: 'v0',
    f1: '6.6%',
    precision: '14.8%',
    precisionFraction: '64/432',
    recall: '4.3%',
    recallFraction: '64/1505',
    tags: ['Closed', 'Reproduced']
  },
  {
    rank: 7,
    medal: undefined,
    model: 'GLM-4.7',
    provider: 'Zhipu AI',
    source: 'Claude Code + Skills',
    sourceType: 'claude',
    version: 'v0',
    f1: '6.6%',
    precision: '11.2%',
    precisionFraction: '70/623',
    recall: '4.7%',
    recallFraction: '70/1505',
    tags: ['Closed', 'Reproduced']
  },
  {
    rank: 8,
    medal: undefined,
    model: 'Deepseek-V3.2',
    provider: 'DeepSeek',
    source: 'Claude Code + Skills',
    sourceType: 'claude',
    version: 'v0',
    f1: '6.4%',
    precision: '10.4%',
    precisionFraction: '69/661',
    recall: '4.6%',
    recallFraction: '69/1505',
    tags: ['Closed', 'Reproduced']
  }
];

const medalIcons: Record<string, string> = {
  gold: '🥇',
  silver: '🥈',
  bronze: '🥉'
};

const BenchmarkSection: React.FC = () => {
  const [hoveredRow, setHoveredRow] = useState<number | null>(null);
  const { t } = useTranslation();

  return (
    <section id="benchmark" className="py-24 relative noise-overlay">
      {/* Ambient glow */}
      <div className="absolute top-1/2 left-1/2 -translate-x-1/2 w-[800px] h-[600px] rounded-full bg-brand-500/[0.02] blur-[140px] pointer-events-none"></div>

      <div className="relative z-10">
        <div className="section-divider mb-24"></div>
        <div className="max-w-7xl mx-auto px-6">
          <div className="text-center mb-12">
            <p className="text-slate-500 text-sm font-mono uppercase tracking-widest mb-3">{t('benchmark.sectionLabel')}</p>
            <h2 className="text-4xl font-bold text-white mb-4">
              {t('benchmark.title')}
            </h2>
            <p className="text-slate-400 max-w-2xl mx-auto">
              {t('benchmark.subtitle')}
            </p>
          </div>

          {/* Legend */}
          <div className="mb-6 flex items-center gap-4 flex-wrap">
            <div className="flex items-center gap-2 text-xs">
              <span className="w-3 h-3 rounded-sm bg-brand-500/20 border border-brand-500/40 inline-block"></span>
              <span className="text-slate-400">{t('benchmark.legendOcr')}</span>
            </div>
            <div className="flex items-center gap-2 text-xs">
              <span className="w-3 h-3 rounded-sm bg-slate-700 border border-slate-600 inline-block"></span>
              <span className="text-slate-400">{t('benchmark.legendClaude')}</span>
            </div>
          </div>

          {/* Table */}
          <div className="rounded-2xl overflow-hidden glass-strong gradient-border shadow-2xl shadow-black/30">
            {/* Table header */}
            <div className="grid grid-cols-12 gap-2 px-6 py-3 bg-dark-700/60 text-xs font-medium text-slate-500 uppercase tracking-wider">
              <div className="col-span-1">{t('benchmark.colRank')}</div>
              <div className="col-span-3">{t('benchmark.colModel')}</div>
              <div className="col-span-2">{t('benchmark.colSource')}</div>
              <div className="col-span-1">{t('benchmark.colVersion')}</div>
              <div className="col-span-2 text-right">SEM.F1</div>
              <div className="col-span-2 text-right">SEM.PRECISION</div>
              <div className="col-span-1 text-right">SEM.RECALL</div>
            </div>

            {leaderboardData.map((entry) => (
              <div
                key={entry.rank}
                className={`leaderboard-row grid grid-cols-12 gap-2 px-6 py-4 items-center cursor-default ${
                  entry.sourceType === 'ocr' ? 'bg-brand-500/3' : ''
                } ${hoveredRow === entry.rank ? 'bg-brand-500/6' : ''}`}
                onMouseEnter={() => setHoveredRow(entry.rank)}
                onMouseLeave={() => setHoveredRow(null)}
              >
                <div className="col-span-1 flex items-center gap-2">
                  {entry.medal ? (
                    <span className="text-lg">{medalIcons[entry.medal]}</span>
                  ) : (
                    <span className="text-slate-500 font-mono text-sm w-6 text-center">{entry.rank}</span>
                  )}
                </div>

                <div className="col-span-3">
                  <div className="text-white text-sm font-medium">{entry.model}</div>
                  <div className="text-slate-500 text-xs">{entry.provider}</div>
                </div>

                <div className="col-span-2">
                  <span
                    className={`px-2 py-0.5 rounded-md text-xs font-medium ${
                      entry.sourceType === 'ocr'
                        ? 'bg-brand-500/15 text-brand-400 border border-brand-500/25'
                        : 'bg-slate-700/50 text-slate-400 border border-slate-600/40'
                    }`}
                  >
                    {entry.source}
                  </span>
                </div>

                <div className="col-span-1 text-slate-500 text-xs font-mono">{entry.version}</div>

                <div className="col-span-2 text-right">
                  <span className={`text-sm font-bold ${entry.rank <= 3 ? 'text-brand-400' : 'text-white'}`}>
                    {entry.f1}
                  </span>
                </div>

                <div className="col-span-2 text-right">
                  <div className="text-slate-300 text-sm">{entry.precision}</div>
                  <div className="text-slate-600 text-xs font-mono">{entry.precisionFraction}</div>
                </div>

                <div className="col-span-1 text-right">
                  <div className="text-slate-300 text-sm">{entry.recall}</div>
                  <div className="text-slate-600 text-xs font-mono">{entry.recallFraction}</div>
                </div>
              </div>
            ))}
          </div>

          <div className="mt-6 text-center text-slate-600 text-xs">
            {t('benchmark.footer')}
          </div>
        </div>
      </div>
    </section>
  );
};

export default BenchmarkSection;
