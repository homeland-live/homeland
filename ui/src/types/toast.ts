import type { Component } from 'vue';

export type ToastType = 'default' | 'info' | 'success' | 'warning' | 'error';

export interface Toast {
  title: string;
  subtitle?: string;
  body: string;
  type?: ToastType;
  icon?: Component;
}
