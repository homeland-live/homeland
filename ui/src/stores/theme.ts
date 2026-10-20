import { defineStore } from 'pinia';
import { LocalStore } from '@/util/store';

const ls = new LocalStore('theme');

export type ThemeMode = 'light' | 'dark' | 'auto';

function prefersDark(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-color-scheme: dark)').matches
  );
}

function readMode(): ThemeMode {
  const v = ls.readS('mode', 'auto');
  return v === 'light' || v === 'dark' || v === 'auto' ? v : 'auto';
}

export type State = {
  mode: ThemeMode;
};

export const useThemeStore = defineStore('theme', {
  state: (): State => ({
    mode: readMode(),
  }),
  getters: {
    isDark(state): boolean {
      if (state.mode === 'auto') {
        return prefersDark();
      }
      return state.mode === 'dark';
    },
  },
  actions: {
    setMode(mode: ThemeMode) {
      this.mode = mode;
      ls.writeS('mode', mode);
    },
    toggle() {
      // Toggle relative to what is currently shown, collapsing "auto" to an explicit choice.
      this.setMode(this.isDark ? 'light' : 'dark');
    },
  },
});
