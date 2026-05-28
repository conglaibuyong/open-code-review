import React, { useState, useEffect } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from '../i18n';
import logoSvg from '../../logo.svg';

const Navbar: React.FC = () => {
  const [isScrolled, setIsScrolled] = useState(false);
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
  const location = useLocation();
  const navigate = useNavigate();
  const isDocsPage = location.pathname === '/docs';
  const { t, language, setLanguage } = useTranslation();

  const navItems = [
    { label: t('navbar.features'), id: 'features' },
    { label: t('navbar.benchmark'), id: 'benchmark' },
    { label: t('navbar.quickstart'), id: 'quickstart' }
  ];

  const navigateToSection = (sectionId: string) => {
    navigate('/', { state: { scrollTo: sectionId } });
    setIsMobileMenuOpen(false);
  };

  useEffect(() => {
    const handleScroll = () => setIsScrolled(window.scrollY > 20);
    window.addEventListener('scroll', handleScroll);
    return () => window.removeEventListener('scroll', handleScroll);
  }, []);

  const scrollToSection = (sectionId: string) => {
    const element = document.getElementById(sectionId);
    if (element) {
      element.scrollIntoView({ behavior: 'smooth' });
    }
    setIsMobileMenuOpen(false);
  };

  const toggleLanguage = () => {
    setLanguage(language === 'en' ? 'zh' : 'en');
  };

  return (
    <nav
      className={`fixed top-0 left-0 right-0 z-50 transition-all duration-300 ${
        isScrolled || isDocsPage
          ? 'bg-dark-900/80 backdrop-blur-xl border-b border-dark-600/30 shadow-lg shadow-black/20'
          : 'bg-transparent'
      }`}
    >
      {/* Top edge glow */}
      <div className="absolute bottom-0 left-0 right-0 h-px bg-gradient-to-r from-transparent via-brand-500/10 to-transparent opacity-0 group-hover:opacity-100"></div>

      <div className="max-w-7xl mx-auto px-6 py-4 flex items-center justify-between">
        <Link to="/" className="flex items-center gap-3">
          <img src={logoSvg} alt="OpenCodeReview" className="w-8 h-8 rounded-lg" />
          <span className="font-bold text-lg tracking-tight">
            <span className="text-white">Open Code Review</span>
          </span>
        </Link>

        <div className="hidden md:flex items-center gap-8">
          {navItems.map((item) => (
            <button
              key={item.id}
              onClick={() => isDocsPage ? navigateToSection(item.id) : scrollToSection(item.id)}
              className="nav-link text-slate-400 hover:text-white text-sm font-medium transition-colors"
            >
              {item.label}
            </button>
          ))}
          <Link
            to="/docs"
            className={`nav-link text-sm font-medium transition-colors flex items-center gap-1 ${
              isDocsPage ? 'text-brand-400' : 'text-slate-400 hover:text-white'
            }`}
          >
            <i className="fa-solid fa-book text-xs"></i>
            {t('navbar.docs')}
          </Link>
        </div>

        <div className="hidden md:flex items-center gap-3">
          <button
            onClick={toggleLanguage}
            className="text-slate-400 hover:text-white text-sm font-medium px-3 py-1.5 rounded-lg border border-dark-600/30 hover:border-slate-500/50 transition-all"
          >
            {language === 'en' ? '中文' : 'EN'}
          </button>
          <a
            href="https://github.com/alibaba/open-code-review"
            target="_blank"
            rel="noopener noreferrer"
            className="btn-secondary text-brand-400 text-sm font-medium px-4 py-2 rounded-lg flex items-center gap-2"
          >
            <i className="fa-brands fa-github"></i>
            GitHub
          </a>
          <button
            onClick={() => isDocsPage ? navigateToSection('quickstart') : scrollToSection('quickstart')}
            className="btn-primary text-dark-900 text-sm font-semibold px-4 py-2 rounded-lg"
          >
            {t('navbar.getStarted')}
          </button>
        </div>

        <button
          className="md:hidden text-slate-400 hover:text-white"
          onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)}
        >
          <i className={`fa-solid ${isMobileMenuOpen ? 'fa-xmark' : 'fa-bars'} text-lg`}></i>
        </button>
      </div>

      {isMobileMenuOpen && (
        <div className="md:hidden bg-dark-900/95 backdrop-blur-xl border-b border-dark-600/30 px-6 py-4 flex flex-col gap-4">
          {navItems.map((item) => (
            <button
              key={item.id}
              onClick={() => isDocsPage ? navigateToSection(item.id) : scrollToSection(item.id)}
              className="text-slate-400 hover:text-white text-sm font-medium text-left transition-colors"
            >
              {item.label}
            </button>
          ))}
          <Link
            to="/docs"
            onClick={() => setIsMobileMenuOpen(false)}
            className={`text-sm font-medium text-left transition-colors flex items-center gap-1 ${
              isDocsPage ? 'text-brand-400' : 'text-slate-400 hover:text-white'
            }`}
          >
            <i className="fa-solid fa-book text-xs"></i>
            {t('navbar.docs')}
          </Link>
          <button
            onClick={toggleLanguage}
            className="text-slate-400 hover:text-white text-sm font-medium text-left transition-colors"
          >
            {language === 'en' ? '切换到中文' : 'Switch to English'}
          </button>
        </div>
      )}
    </nav>
  );
};

export default Navbar;
