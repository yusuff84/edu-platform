import ru from './ru.json';

// Типобезопасный i18n-словарь.
// RuKey выводится из ru.json (typeof ru — объект default-импорта JSON-модуля,
// то же самое, что typeof import('./ru.json')['default']): рекурсивный ключ-тип
// собирает все листья словаря,
// склеивая уровни вложенности точкой. Поэтому работают и плоские ключи
// ("nav.catalog"), и потенциально вложенные ({ nav: { catalog } }).
type PathsOf<T> = T extends string
  ? never
  : {
      [K in keyof T & (string | number)]: T[K] extends string
        ? K
        : `${K}.${PathsOf<T[K]>}`;
    }[keyof T & (string | number)];

export type RuKey = PathsOf<typeof ru>;

/**
 * Перевод ключа словаря. Принимает только ключи, реально существующие
 * в ru.json: опечатка в ключе — ошибка компиляции.
 */
export function t(key: RuKey): string {
  // Словарь сейчас плоский; обход по точкам поддерживает и будущую вложенность.
  let node: unknown = ru;
  for (const part of key.split('.')) {
    if (typeof node !== 'object' || node === null) return key;
    node = (node as Record<string, unknown>)[part];
  }
  return typeof node === 'string' ? node : key;
}

export default ru;
