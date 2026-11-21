import type { QueryClient } from '@tanstack/react-query';
import type { User } from '../api/types';

export async function setSessionUser(client: QueryClient, user: User | null): Promise<void> {
  await client.cancelQueries();
  // Preserve the observed identity query so shell/guards see the new session
  // immediately. All other queries may contain account-specific fields.
  client.removeQueries({ predicate: query => !(query.queryKey.length === 1 && query.queryKey[0] === 'me') });
  client.setQueryData(['me'], user);
}
