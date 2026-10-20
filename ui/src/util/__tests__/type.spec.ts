import { describe, it, expect } from 'vitest';
// prettier-ignore
import {
  isBoolean,
  isString,
  isNumber,
  isDate,
  isRegExp,
  isFunction,
} from '@/util/type';

describe('type', () => {
  const boolValue = false;
  const strValue = 'foobar';
  const numValue = 42;
  const dateValue = new Date();
  const reValue = /\\w+/;
  const fnValue = function noop() {};

  describe('isBoolean', () => {
    it('checks if arg is Boolean', () => {
      expect(isBoolean(boolValue)).toBe(true);
      expect(isBoolean(strValue)).toBe(false);
      expect(isBoolean(numValue)).toBe(false);
      expect(isBoolean(dateValue)).toBe(false);
      expect(isBoolean(reValue)).toBe(false);
      expect(isBoolean(fnValue)).toBe(false);
    });
  });

  describe('isString', () => {
    it('checks if arg is String', () => {
      expect(isString(boolValue)).toBe(false);
      expect(isString(strValue)).toBe(true);
      expect(isString(numValue)).toBe(false);
      expect(isString(dateValue)).toBe(false);
      expect(isString(reValue)).toBe(false);
      expect(isString(fnValue)).toBe(false);
    });
  });

  describe('isNumber', () => {
    it('checks if arg is Number', () => {
      expect(isNumber(boolValue)).toBe(false);
      expect(isNumber(strValue)).toBe(false);
      expect(isNumber(numValue)).toBe(true);
      expect(isNumber(dateValue)).toBe(false);
      expect(isNumber(reValue)).toBe(false);
      expect(isNumber(fnValue)).toBe(false);
    });
  });

  describe('isDate', () => {
    it('checks if arg is Date', () => {
      expect(isDate(boolValue)).toBe(false);
      expect(isDate(strValue)).toBe(false);
      expect(isDate(numValue)).toBe(false);
      expect(isDate(dateValue)).toBe(true);
      expect(isDate(reValue)).toBe(false);
      expect(isDate(fnValue)).toBe(false);
    });
  });

  describe('isRegExp', () => {
    it('checks if arg is Regexp', () => {
      expect(isRegExp(boolValue)).toBe(false);
      expect(isRegExp(strValue)).toBe(false);
      expect(isRegExp(numValue)).toBe(false);
      expect(isRegExp(dateValue)).toBe(false);
      expect(isRegExp(reValue)).toBe(true);
      expect(isRegExp(fnValue)).toBe(false);
    });
  });

  describe('isFunction', () => {
    it('checks if arg is isFunction', () => {
      expect(isFunction(boolValue)).toBe(false);
      expect(isFunction(strValue)).toBe(false);
      expect(isFunction(numValue)).toBe(false);
      expect(isFunction(dateValue)).toBe(false);
      expect(isFunction(reValue)).toBe(false);
      expect(isFunction(fnValue)).toBe(true);
    });
  });
});
