// Relais même origine vers l'API interne : le navigateur n'appelle que le front,
// sans CORS ni dépendance à localhost:8083 (accès par IP, Local Network Access).
// Liste blanche stricte, lecture seule ; seuls la requête et l'en-tête de
// pagination sont transmis.
const ALLOWED = new Set(["graph", "data"]);
const PASS_HEADERS = ["content-type", "x-next-cursor"];

export default defineEventHandler(async (event) => {
  const path = getRouterParam(event, "path") ?? "";
  if (!ALLOWED.has(path)) {
    throw createError({ statusCode: 404, statusMessage: "Not Found" });
  }

  const { apiInternal } = useRuntimeConfig(event);
  const target = new URL(`/${path}`, apiInternal as string);
  for (const [key, value] of Object.entries(getQuery(event))) {
    if (typeof value === "string") target.searchParams.set(key, value);
  }

  let upstream: Response;
  try {
    upstream = await fetch(target, {
      headers: { accept: "application/json" },
      signal: AbortSignal.timeout(7000),
    });
  } catch {
    throw createError({ statusCode: 502, statusMessage: "API injoignable" });
  }

  setResponseStatus(event, upstream.status);
  for (const name of PASS_HEADERS) {
    const value = upstream.headers.get(name);
    if (value) setResponseHeader(event, name, value);
  }
  setResponseHeader(event, "Cache-Control", "no-store");
  return upstream.text();
});
