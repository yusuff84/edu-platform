import ru from './ru.json';

export type RuKey = keyof typeof ru;

export function t(key: string): string {
  const dict = ru as Record<string, string>;
  return dict[key] ?? key;
}

export default ru;
