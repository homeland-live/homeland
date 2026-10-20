import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useToastStore } from '@/stores/toast';

beforeEach(() => {
  setActivePinia(createPinia());
});

describe('toast store', () => {
  it('enqueues a toast with an incrementing id and returns it', () => {
    const store = useToastStore();
    const id1 = store.show({ title: 'a', body: '' });
    const id2 = store.show({ title: 'b', body: '' });
    expect(id1).not.toBe(id2);
    expect(store.toasts).toHaveLength(2);
    expect(store.toasts[0]?.id).toBe(id1);
  });

  it('success/error shortcuts set the type', () => {
    const store = useToastStore();
    store.success('ok', 'done');
    store.error('bad', 'failed');
    expect(store.toasts[0]).toMatchObject({ title: 'ok', body: 'done', type: 'success' });
    expect(store.toasts[1]).toMatchObject({ title: 'bad', body: 'failed', type: 'error' });
  });

  it('dismiss removes a toast by id and is a no-op for unknown ids', () => {
    const store = useToastStore();
    const id = store.success('ok');
    store.dismiss(999);
    expect(store.toasts).toHaveLength(1);
    store.dismiss(id);
    expect(store.toasts).toHaveLength(0);
  });

  it('applies the default timeout and honors an explicit one', () => {
    const store = useToastStore();
    store.success('a');
    store.show({ title: 'b', body: '' }, 0);
    expect(store.toasts[0]?.timeout).toBe(4000);
    expect(store.toasts[1]?.timeout).toBe(0);
  });
});
