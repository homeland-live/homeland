import { defineStore } from 'pinia';
import type { Toast } from '@/types/toast';

// defaultTimeout is how long a toast stays visible before auto-dismissing, in
// milliseconds. A timeout of 0 keeps the toast until dismissed explicitly.
const defaultTimeout = 4000;

// ActiveToast is a queued toast with the identity and timeout the host needs to
// render and auto-dismiss it. `id` is assigned by the store.
export interface ActiveToast extends Toast {
  id: number;
  timeout: number;
}

export type State = {
  toasts: ActiveToast[];
  nextId: number;
};

export const useToastStore = defineStore('toast', {
  state: (): State => ({
    toasts: [],
    nextId: 1,
  }),
  actions: {
    // show enqueues a toast and returns its id. timeout defaults to
    // defaultTimeout; pass 0 to keep it until dismissed.
    show(toast: Toast, timeout: number = defaultTimeout): number {
      const id = this.nextId;
      this.nextId += 1;
      this.toasts.push({ ...toast, id, timeout });
      return id;
    },
    // success is a shortcut for a success-type toast.
    success(title: string, body = '', timeout: number = defaultTimeout): number {
      return this.show({ title, body, type: 'success' }, timeout);
    },
    // error is a shortcut for an error-type toast.
    error(title: string, body = '', timeout: number = defaultTimeout): number {
      return this.show({ title, body, type: 'error' }, timeout);
    },
    // dismiss removes a toast by id (no-op if it is already gone).
    dismiss(id: number) {
      this.toasts = this.toasts.filter((t) => t.id !== id);
    },
  },
});
