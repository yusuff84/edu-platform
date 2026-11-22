import type { FieldErrors, FieldValues, Resolver } from 'react-hook-form';

/**
 * Мини-resolver для react-hook-form вместо zod + @hookform/resolvers
 * (zod весил ≈125 КБ и тянулся на страницу курса ради формы отзыва).
 *
 * validate получает сырые значения формы и возвращает либо преобразованные
 * значения (то, что уйдёт в onSubmit — например, с обрезанными пробелами),
 * либо сообщения об ошибках по полям. Для каждого поля показывается первая
 * не пройденная проверка — как было с zodResolver (criteriaMode: firstError).
 */
type ValidationResult<TIn, TOut> =
  | { values: TOut }
  | { errors: Partial<Record<keyof TIn & string, string>> };

export function makeResolver<TIn extends FieldValues, TOut extends FieldValues>(
  validate: (values: TIn) => ValidationResult<TIn, TOut>,
): Resolver<TIn, unknown, TOut> {
  return (values, _context, options) => {
    const result = validate(values);
    if ('values' in result) return { values: result.values, errors: {} };
    const errors: Record<string, unknown> = {};
    for (const [name, message] of Object.entries(result.errors)) {
      if (!message) continue;
      const field = options.fields[name];
      errors[name] = { type: 'validate', message, ref: field?.refs ? field.refs[0] : field?.ref };
    }
    if (Object.keys(errors).length === 0) return { values: values as unknown as TOut, errors: {} };
    return { values: {}, errors: errors as FieldErrors<TIn> };
  };
}

/**
 * Первая не пройденная проверка: пары [условие ошибки, сообщение] по порядку.
 * Возвращает сообщение или undefined, если всё в порядке.
 */
export function firstError(...checks: Array<[boolean, string]>): string | undefined {
  for (const [failed, message] of checks) if (failed) return message;
  return undefined;
}

/** Длина строки в символах Unicode (суррогатная пара — один символ), как в zod 4. */
export function textLength(value: string): number {
  return Array.from(value).length;
}

/**
 * Приведение к числу как у z.coerce.number(): Number(value); NaN/±Infinity —
 * ошибка типа с тем же текстом, что выдавал zod.
 */
export function coerceNumber(value: unknown): { ok: true; value: number } | { ok: false; message: string } {
  const n = Number(value);
  if (Number.isFinite(n)) return { ok: true, value: n };
  return { ok: false, message: `Invalid input: expected number, received ${String(n)}` };
}

/** Проверка email из zod 4 (z.string().email()) — тот же регэксп. */
export const EMAIL_RE =
  /^(?:[A-Za-z0-9_'+\-]+\.)*[A-Za-z0-9_'+\-]*[A-Za-z0-9_+-]@(?:[A-Za-z0-9][A-Za-z0-9\-]*\.)+[A-Za-z]{2,}$/;
