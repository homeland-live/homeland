export const DEFAULT_LOCALE = 'en-US';
export const DEFAULT_CURRENCY = 'EUR';
export const DEFAULT_PRECISION = 2;

export interface FormatterOpts {
  locale: string;
  currency: string;
  precision: number;
}

export class Formatter {
  opts: FormatterOpts;

  currencyFormatter: Intl.NumberFormat;

  percentFormatter: Intl.NumberFormat;

  constructor(opts: Partial<FormatterOpts> = {}) {
    this.opts = {
      locale: DEFAULT_LOCALE,
      currency: DEFAULT_CURRENCY,
      precision: DEFAULT_PRECISION,
      ...opts,
    };

    this.currencyFormatter = new Intl.NumberFormat(this.opts.locale, {
      style: 'currency',
      currency: this.opts.currency,
      minimumFractionDigits: this.opts.precision,
    });

    this.percentFormatter = new Intl.NumberFormat(this.opts.locale, {
      style: 'percent',
      minimumFractionDigits: this.opts.precision,
    });
  }

  currency(num: number): string {
    return this.currencyFormatter.format(num);
  }

  percent(num: number): string {
    return this.percentFormatter.format(num / 100);
  }

  round(num: number): string {
    return this.percent(num).replace('%', '');
  }
}

export function roundFloat(num: number, precision: number): number {
  const ratio = 10 ** precision;
  return Math.round(num * ratio) / ratio;
}

// percentOf calculates what percent of `part` from `total`
export function percentOf(part: number, total: number): number {
  if (total === 0) {
    return 0;
  }
  return roundFloat((100 * part) / total, 4);
}

// percentDiff calculates percent difference between two numbers
export function percentDiff(before: number, after: number): number {
  return percentOf(after - before, before);
}

// changeByPercent calculates change of given number by given percent
export function changeByPercent(value: number, percent: number): number {
  // prettier-ignore
  return value + (value * (percent / 100));
}

// drawdown calculates percentage of the peak value
export function drawdown(high: number, regular: number): number {
  return percentOf(high - regular, high);
}
