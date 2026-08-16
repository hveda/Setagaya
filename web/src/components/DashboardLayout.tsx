import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { Menu, Moon, Sun, X } from 'lucide-react';
import Button from './ui/Button';

const navItems = [
  { href: '/reports', label: 'Reports' },
  { href: '/reservations', label: 'Reservations' },
  { href: '/status', label: 'Live Status' },
  { href: '/campaigns', label: 'Campaigns' },
];

function applyTheme(dark: boolean) {
  document.documentElement.classList.toggle('dark', dark);
}

export default function DashboardLayout({ children }: { children: ReactNode }) {
  const location = useLocation();
  const [isDark, setIsDark] = useState(false);
  const [mounted, setMounted] = useState(false);
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);

  useEffect(() => {
    const savedTheme = localStorage.getItem('honryu-theme');
    const shouldBeDark = savedTheme ? savedTheme === 'dark' : window.matchMedia('(prefers-color-scheme: dark)').matches;
    setIsDark(shouldBeDark);
    applyTheme(shouldBeDark);
    setMounted(true);
  }, []);

  const toggleTheme = () => {
    const next = !isDark;
    setIsDark(next);
    localStorage.setItem('honryu-theme', next ? 'dark' : 'light');
    applyTheme(next);
  };

  if (!mounted) {
    return null;
  }

  return (
    <div className="min-h-screen bg-white text-slate-700 transition-colors duration-300 dark:bg-slate-950 dark:text-slate-300">
      <nav className="sticky top-0 z-40 border-b border-slate-200/70 bg-white/80 backdrop-blur-md dark:border-slate-800/70 dark:bg-slate-950/80">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="flex h-16 items-center justify-between">
            <div className="flex items-center space-x-8">
              <Link to="/" className="text-xl font-bold tracking-tight text-slate-900 dark:text-white">
                Honryu<span className="text-sky-500">.</span>
              </Link>
              <div className="hidden space-x-2 md:flex">
                {navItems.map((item) => (
                  <Link
                    key={item.href}
                    to={item.href}
                    className={`flex min-h-[44px] items-center rounded-md px-3 py-2 text-sm font-medium transition-colors duration-200 ${
                      location.pathname === item.href
                        ? 'bg-sky-50 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300'
                        : 'text-slate-600 hover:bg-slate-100 hover:text-sky-600 dark:text-slate-300 dark:hover:bg-slate-800 dark:hover:text-sky-400'
                    }`}
                  >
                    {item.label}
                  </Link>
                ))}
              </div>
            </div>

            <div className="hidden items-center space-x-4 md:flex">
              <Button
                onClick={toggleTheme}
                variant="ghost"
                size="md"
                className="min-h-[44px] min-w-[44px] p-2 text-amber-500 dark:text-sky-600"
                title="Toggle theme"
              >
                {isDark ? <Sun className="h-5 w-5" /> : <Moon className="h-5 w-5" />}
              </Button>
            </div>

            <div className="flex items-center space-x-2 md:hidden">
              <Button
                onClick={toggleTheme}
                variant="ghost"
                size="md"
                className="min-h-[44px] min-w-[44px] p-2 text-amber-500 dark:text-sky-600"
                title="Toggle theme"
              >
                {isDark ? <Sun className="h-5 w-5" /> : <Moon className="h-5 w-5" />}
              </Button>
              <Button
                onClick={() => setIsMobileMenuOpen((open) => !open)}
                variant="ghost"
                size="md"
                className="min-h-[44px] min-w-[44px] p-2 text-slate-600 dark:text-slate-300"
                title="Toggle menu"
              >
                {isMobileMenuOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
              </Button>
            </div>
          </div>
        </div>

        <div
          className={`transform border-b border-slate-200 bg-white/95 backdrop-blur-md transition-all duration-300 ease-in-out dark:border-slate-800 dark:bg-slate-950/95 md:hidden ${
            isMobileMenuOpen ? 'translate-y-0 opacity-100' : 'pointer-events-none invisible -translate-y-2 opacity-0'
          }`}
        >
          <div className="space-y-2 px-4 py-4">
            {navItems.map((item) => (
              <Link
                key={item.href}
                to={item.href}
                onClick={() => setIsMobileMenuOpen(false)}
                className={`block min-h-[44px] rounded-lg px-4 py-3 text-base font-medium transition-colors duration-200 ${
                  location.pathname === item.href
                    ? 'bg-sky-50 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300'
                    : 'text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800'
                }`}
              >
                {item.label}
              </Link>
            ))}
          </div>
        </div>
      </nav>

      <main className="relative z-10 mx-auto max-w-7xl px-4 py-4 sm:px-6 sm:py-6 lg:px-8 lg:py-8">{children}</main>
    </div>
  );
}
