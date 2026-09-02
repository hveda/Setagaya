import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { Menu, Moon, Sun, X } from 'lucide-react';
import Button from './ui/Button';

const navItems = [
  { href: '/reports', label: 'Reports' },
  { href: '/executions', label: 'Executions' },
  { href: '/reservations', label: 'Reservations' },
  { href: '/campaigns', label: 'Campaigns' },
  { href: '/clusters', label: 'Clusters' },
];

function applyTheme(dark: boolean) {
  document.documentElement.classList.toggle('dark', dark);
}

/**
 * Classes for the mobile nav drawer, exported so the layout invariant is
 * testable without a layout engine (jsdom computes no geometry).
 *
 * The invariant: the drawer is `absolute` in BOTH states. A closed drawer
 * that participates in normal flow still reserves its full height even when
 * `invisible` -- that reserved band pushed <main> down ~306px on every page
 * (nav measured 370px against h-16's 64px), which read on a phone as a
 * screenful of empty space above the content. Positioning it out of flow is
 * what fixes that, so `absolute` is the thing worth pinning down.
 */
export function mobileMenuClasses(isOpen: boolean): string {
  const base =
    'mobile-menu absolute top-full right-0 left-0 transform border-b border-slate-200 bg-white/95 shadow-lg backdrop-blur-md transition-all duration-300 ease-in-out dark:border-slate-800 dark:bg-slate-950/95 md:hidden';
  // pointer-events-none/invisible: while closed the drawer still overlaps the
  // nav bar, so without these its links swallow taps meant for the burger
  // button. -translate-y-full (not -2) slides it fully behind the bar.
  const state = isOpen ? 'translate-y-0 opacity-100' : 'pointer-events-none invisible -translate-y-full opacity-0';
  return `${base} ${state}`;
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

  // Tapping anywhere outside closes the open drawer. Without this the only
  // way out is the X, since the drawer overlays the page rather than pushing
  // it down -- tapping the content you can see underneath would otherwise do
  // nothing. Matched on marker classes rather than refs so the burger button
  // itself is excluded: it owns its own toggle, and letting this handler see
  // it too would close-then-reopen on a single tap.
  useEffect(() => {
    if (!isMobileMenuOpen) {
      return;
    }
    const handleClickOutside = (event: MouseEvent) => {
      const target = event.target as Element | null;
      if (!target?.closest('.mobile-menu') && !target?.closest('.mobile-menu-button')) {
        setIsMobileMenuOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [isMobileMenuOpen]);

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
                className="mobile-menu-button min-h-[44px] min-w-[44px] p-2 text-slate-600 dark:text-slate-300"
                title="Toggle menu"
              >
                {isMobileMenuOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
              </Button>
            </div>
          </div>
        </div>

        <div className={mobileMenuClasses(isMobileMenuOpen)}>
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
