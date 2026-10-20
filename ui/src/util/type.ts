function is(arg: unknown, name: string): boolean {
  return Object.prototype.toString.call(arg) === `[object ${name}]`;
}

export function isBoolean(arg: unknown): arg is boolean {
  return is(arg, 'Boolean');
}

export function isString(arg: unknown): arg is string {
  return is(arg, 'String');
}

export function isNumber(arg: unknown): arg is number {
  return is(arg, 'Number');
}

export function isDate(arg: unknown): arg is Date {
  return is(arg, 'Date');
}

export function isRegExp(arg: unknown): arg is RegExp {
  return is(arg, 'RegExp');
}

export function isFunction(arg: unknown): boolean {
  return is(arg, 'Function');
}
