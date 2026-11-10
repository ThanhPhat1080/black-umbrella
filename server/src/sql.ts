export interface Sql {
  exec<T = Record<string, unknown>>(query: string, ...params: unknown[]): T[];
}
