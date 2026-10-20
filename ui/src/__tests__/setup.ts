// Test environment polyfills for Vuetify under jsdom.
// jsdom provides neither ResizeObserver nor matchMedia, both of which
// Vuetify's layout and display composables rely on.

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

globalThis.ResizeObserver ??= ResizeObserverStub as unknown as typeof ResizeObserver;

if (typeof window !== 'undefined' && !window.matchMedia) {
  window.matchMedia = (query: string): MediaQueryList =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }) as unknown as MediaQueryList;
}

if (typeof window !== 'undefined') {
  window.visualViewport ??= {
    width: window.innerWidth,
    height: window.innerHeight,
    addEventListener: () => {},
    removeEventListener: () => {},
  } as unknown as VisualViewport;
}
