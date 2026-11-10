export interface Sql {
  exec<T = Record<string, unknown>>(query: string, ...params: unknown[]): T[];
  /** Runs fn atomically (synchronously): commits when it returns, rolls back when it throws. */
  tx<T>(fn: () => T): T;
}
