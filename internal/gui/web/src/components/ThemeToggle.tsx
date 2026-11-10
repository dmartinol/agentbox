import { useEffect, useState } from 'react';
import { Moon, Sun } from 'lucide-react';
import { Button } from './ui/button';

const storageKey = 'agentbox-theme';
export type Theme = 'light' | 'dark';

function initialTheme(): Theme {
  if (typeof window === 'undefined') return 'light';
  return window.localStorage.getItem(storageKey) === 'dark' ? 'dark' : 'light';
}

export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(initialTheme);

  useEffect(() => {
    document.documentElement.classList.toggle('theme-light', theme === 'light');
    window.localStorage.setItem(storageKey, theme);
  }, [theme]);

  const next = theme === 'light' ? 'dark' : 'light';
  return (
    <Button
      size="icon"
      variant="ghost"
      aria-label={`Switch to ${next} theme`}
      title={`Switch to ${next} theme`}
      onClick={() => setTheme(next)}
    >
      {theme === 'light' ? <Moon className="size-4" /> : <Sun className="size-4" />}
    </Button>
  );
}
