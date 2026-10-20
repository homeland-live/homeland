import { describe, it, expect } from 'vitest';
import {
  Formatter,
  roundFloat,
  percentOf,
  percentDiff,
  changeByPercent,
  drawdown,
} from '@/util/num';

describe('num', () => {
  describe('Formatter', () => {
    describe('currency', () => {
      it('formats currency with default precision', () => {
        const fmt = new Formatter();
        expect(fmt.currency(0)).toEqual('€0.00');
        expect(fmt.currency(-1.1)).toEqual('-€1.10');
        expect(fmt.currency(1234.5678)).toEqual('€1,234.57');
      });

      it('formats currency with custom precision', () => {
        const fmt = new Formatter({ precision: 3 });
        expect(fmt.currency(0)).toEqual('€0.000');
        expect(fmt.currency(-1.1)).toEqual('-€1.100');
        expect(fmt.currency(1234.5678)).toEqual('€1,234.568');
      });
    });

    describe('percent', () => {
      it('formats percent with default precision', () => {
        const fmt = new Formatter();
        expect(fmt.percent(0)).toEqual('0.00%');
        expect(fmt.percent(-1.1)).toEqual('-1.10%');
        expect(fmt.percent(1234.5678)).toEqual('1,234.57%');
      });

      it('formats percent with custom precision', () => {
        const fmt = new Formatter({ precision: 3 });
        expect(fmt.percent(0)).toEqual('0.000%');
        expect(fmt.percent(-1.1)).toEqual('-1.100%');
        expect(fmt.percent(1234.5678)).toEqual('1,234.568%');
      });
    });

    describe('round', () => {
      it('rounds number with default precision', () => {
        const fmt = new Formatter();
        expect(fmt.round(0)).toEqual('0.00');
        expect(fmt.round(-1.1)).toEqual('-1.10');
        expect(fmt.round(1234.5678)).toEqual('1,234.57');
      });

      it('rounds number with custom precision', () => {
        const fmt = new Formatter({ precision: 3 });
        expect(fmt.round(0)).toEqual('0.000');
        expect(fmt.round(-1.1)).toEqual('-1.100');
        expect(fmt.round(1234.5678)).toEqual('1,234.568');
      });
    });
  });

  describe('roundFloat', () => {
    it('rounds given float number to given precision (decimal digits)', () => {
      expect(roundFloat(12.3456789, 0)).toEqual(12);
      expect(roundFloat(12.3456789, 1)).toEqual(12.3);
      expect(roundFloat(12.3456789, 2)).toEqual(12.35);
      expect(roundFloat(12.3456789, 3)).toEqual(12.346);
      expect(roundFloat(12.3456789, 4)).toEqual(12.3457);
      expect(roundFloat(12.3456789, 5)).toEqual(12.34568);
      expect(roundFloat(12.3456789, 6)).toEqual(12.345679);
      expect(roundFloat(-12.3456789, 4)).toEqual(-12.3457);
    });
  });

  describe('percentOf', () => {
    it('calculates what percent of `part` from `total`', () => {
      expect(percentOf(1, 0)).toEqual(0);
      expect(percentOf(1, 50)).toEqual(2);
      expect(percentOf(1.25, 50)).toEqual(2.5);
      expect(percentOf(33.33, 50)).toEqual(66.66);
      expect(percentOf(49, 50)).toEqual(98);
      expect(percentOf(55.55, 50)).toEqual(111.1);
      expect(percentOf(3, 11)).toEqual(27.2727);
    });
  });

  describe('percentDiff', () => {
    it('calculates percent difference between two numbers', () => {
      expect(percentDiff(1, 1.25)).toEqual(25);
      expect(percentDiff(1.25, 1)).toEqual(-20);
    });
  });

  describe('changeByPercent', () => {
    it('changeByPercent calculates change of given number by given percent', () => {
      expect(changeByPercent(1, 25)).toEqual(1.25);
      expect(changeByPercent(1, -25)).toEqual(0.75);
      expect(changeByPercent(25, 5)).toEqual(26.25);
      expect(changeByPercent(50, -20)).toEqual(40);
    });
  });

  describe('drawdown', () => {
    it('drawdown calculates percentage of the peak value', () => {
      expect(drawdown(40.78, 17.64)).toEqual(56.7435);
      expect(drawdown(40.78, 18.37)).toEqual(54.9534);
      expect(drawdown(90, 45)).toEqual(50);
      expect(drawdown(100, 99)).toEqual(1);
      expect(drawdown(20, 25)).toEqual(-25);
    });
  });
});
