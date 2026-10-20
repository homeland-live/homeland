import { DateTime, type ToRelativeUnit, type StringUnitLength } from 'luxon';

interface Opts {
  unit?: ToRelativeUnit | ToRelativeUnit[];
  style?: StringUnitLength;
}

export function toRelative(dt: DateTime, opts: Opts = {}): string {
  if (!dt.isValid) {
    return 'n/a';
  }
  // prettier-ignore
  const date = opts.unit === 'days' ? dt.toRelativeCalendar({ unit: opts.unit }) : dt.toRelative(opts);
  if (!date) {
    return 'n/a';
  }
  if (date === '0 minutes ago' || date === 'in 0 minutes') {
    return 'just now';
  }
  if (date === 'today') {
    const time = dt.toRelative({ ...opts, unit: ['hours', 'minutes'] });
    if (!time) {
      return date;
    }
    if (time === '0 minutes ago' || time === 'in 0 minutes') {
      return opts.unit === 'days' ? `${date}, now` : 'just now';
    }
    return opts.unit === 'days' ? `${date}, ${time}` : time;
  }
  return date;
}
