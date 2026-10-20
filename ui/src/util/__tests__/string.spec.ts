import { describe, it, expect } from 'vitest';
import { capitalize } from '@/util/string';

describe('string', () => {
  describe('capitalize', () => {
    it('capitalizes any given string', () => {
      expect(capitalize('here we go')).toEqual('Here we go');
      expect(capitalize('foobar')).toEqual('Foobar');
    });
  });
});
