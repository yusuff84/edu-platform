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
  // Словарь сейчас ПЛОСКИЙ: ru.json хранит ключ как цельную строку
  // ("catalog.title": "Каталог курсов"), а не вложенность { catalog: { title } }.
  // Поэтому сначала ищем точное совпадение — иначе обход по точкам уйдёт в
  // ru["catalog"] === undefined и вернёт сырой ключ вместо перевода.
  // Обход по точкам оставлен ниже: он подхватит переход на вложенность
  // ({"catalog": {"title": ...}}) без изменения всех вызовов t().
  const direct = (ru as Record<string, unknown>)[key];
  if (typeof direct === 'string') return direct;
  let node: unknown = ru;
  for (const part of key.split('.')) {
    if (typeof node !== 'object' || node === null) return key;
    node = (node as Record<string, unknown>)[part];
  }
  return typeof node === 'string' ? node : key;
}

export default ru;
