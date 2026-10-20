// Русское склонение существительных при числительных: 1 отзыв, 2 отзыва,
// 5 отзывов. Вынесено отдельно от t(), чтобы подписи-счётчики не дублировали
// формы в ru.json и не зависели от конкретного существительного.
export function pluralRu(n: number, one: string, few: string, many: string): string {
  const abs = Math.abs(Math.trunc(n)) % 100;
  const last = abs % 10;
  if (abs >= 11 && abs <= 14) return many;
  if (last === 1) return one;
  if (last >= 2 && last <= 4) return few;
  return many;
}
