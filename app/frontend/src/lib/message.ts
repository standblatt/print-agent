/** The message of an error a Go call was rejected with. */
export const message = (e: unknown) => (e instanceof Error ? e.message : String(e));
