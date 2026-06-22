# Safe Routes — Frontend Integration Spec

> Contrato de integración del endpoint `GET /api/v1/routes/safe` para el equipo de frontend.
> Refleja el comportamiento **actual** del backend (`internal/saferoutes/`).
>
> **Lenguaje de producto (obligatorio en la UI):** siempre hablar de **"exposición histórica
> estimada al delito"**, nunca de seguridad garantizada. No prometer que una ruta "es segura".

---

## 1. Resumen

Dado un origen y un destino dentro de CABA (y opcionalmente un momento), el backend devuelve hasta
**4 rutas caminables alternativas** que compensan distancia contra exposición histórica estimada al
delito:

| `kind`                 | Qué optimiza |
|------------------------|--------------|
| `fastest`              | Distancia pura (mínima). |
| `balanced`             | Distancia + riesgo, peso moderado. |
| `safest`               | Distancia + riesgo, peso alto (más desvío tolerado). |
| `least_safe_candidate` | **No se optimiza** — la ruta de mayor exposición entre candidatos cercanos. Contraste; **puede no venir**. |

Cada ruta trae su geometría para dibujar, métricas comparativas, y **metadata explicativa** del
porqué de su riesgo.

---

## 2. Endpoint

```
GET {BASE_URL}/api/v1/routes/safe
```

- **Método:** `GET` · **Auth:** ninguna (todavía) · **Respuesta:** `application/json`
- **BASE_URL (dev):** `http://localhost:8080`

### CORS ⚠️

El backend hoy permite **un solo origen**: `http://localhost:8081` (`internal/app/routes.go`). Si el
frontend corre en otro puerto/host, el navegador **bloqueará** las llamadas. Servir el front en
`:8081` en dev, o pedir agregar el origen a `AllowedOrigins` (cambio de una línea, requiere su propio
OpenSpec change).

### Docs vivas y correlación

- OpenAPI: `GET {BASE_URL}/openapi.yaml` · Swagger UI: `GET {BASE_URL}/docs/`
- La respuesta trae el header `X-Request-Id` (adjuntarlo al reportar problemas).

---

## 3. Request — query params

| Param        | Tipo    | Requerido | Restricción |
|--------------|---------|-----------|-------------|
| `origin_lat` | float   | sí        | WGS84, CABA: `-35 ≤ lat ≤ -34`. |
| `origin_lng` | float   | sí        | WGS84, CABA: `-59 ≤ lng ≤ -58`. |
| `dest_lat`   | float   | sí        | idem. |
| `dest_lng`   | float   | sí        | idem. |
| `datetime`   | string  | no        | RFC3339 (ej. `2026-06-12T23:00:00-03:00`). Si se omite, hora actual de `America/Argentina/Buenos_Aires`. |

Reglas (todas → HTTP 400, ver §5): coordenadas presentes y parseables; dentro de CABA; origen ≠
destino; `datetime` RFC3339 válido; y ambos extremos a **≤ 150 m** de la red caminable.

El `datetime` selecciona el **contexto de riesgo**: **franjas** `morning` 06–11 · `afternoon` 12–17 ·
`evening` 18–21 · `night` (resto); y **tipo de día** `weekend` (sáb/dom) · `weekday`.

### Ejemplo

```
GET /api/v1/routes/safe?origin_lat=-34.6037&origin_lng=-58.3816&dest_lat=-34.5805&dest_lng=-58.4205&datetime=2026-06-12T23:00:00-03:00
```

---

## 4. Response 200 — estructura

```jsonc
{
  "origin":       { "lat": -34.6037, "lng": -58.3816 },
  "destination":  { "lat": -34.5805, "lng": -58.4205 },
  "datetime":     "2026-06-12T23:00:00-03:00",
  "time_bucket":  "night",                      // morning|afternoon|evening|night
  "weekday_type": "weekday",                    // weekday|weekend
  "model_version": { "id": 2, "name": "...", "type": "...", "train_until": "2025-12-31" },
  "routes": [ /* 1..4 objetos, ver abajo */ ]
}
```

### Objeto de `routes[]`

```jsonc
{
  "kind": "balanced",                         // fastest|balanced|safest|least_safe_candidate
  "distance_meters": 5900.6,
  "duration_minutes": 70.2,
  "risk_score": 0.47,                         // 0..1
  "risk_level": "moderate",                   // low|moderate|high
  "extra_distance_vs_fastest_meters": 903,    // 0 para fastest
  "extra_duration_vs_fastest_minutes": 10.7,  // 0 para fastest
  "risk_reduction_vs_fastest_percent": 49.9,  // % exposición removida vs fastest; 0 para fastest
  "high_risk_edge_meters": 581.4,
  "high_risk_edge_percent": 9.9,
  "max_edge_risk": 1,
  "avg_edge_risk": 0.293,
  "crime_metrics": {
    "crime_count": 375221, "robbery_count": 147932, "theft_count": 155518,
    "threats_count": 13263, "armed_count": 17942, "motorcycle_count": 15465,
    "same_bucket_crime_count": 74198
  },

  // --- metadata explicativa (todas métricas; el FE compone la prosa) ---
  "riskiest_segment": {                  // la cuadra que dispara el score (== max_edge_risk)
    "risk_score": 1, "risk_level": "high", "length_meters": 62,
    "point": { "lat": -34.5783, "lng": -58.4250 },
    "crime_count": 4202, "robbery_count": 1787, "armed_count": 198,
    "theft_count": 1765, "threats_count": 114, "motorcycle_count": 154
  },
  "segments": [                          // una entrada por cuadra, en orden de recorrido
    { "risk_score": 0.893, "robbery_count": 1508, "length_meters": 54.3,
      "point": { "lat": -34.5783, "lng": -58.4246 } }
    // ...
  ],
  "dominant_factor": "theft",            // "robbery" | "theft" | "threats" | "none"
  "armed_share_percent": 4.3,            // armed_count / crime_count * 100
  "time_of_day_risk": {                  // riesgo de ESTA ruta por franja (weekday_type resuelto)
    "morning":   { "risk_score": 0.744, "risk_level": "high" },
    "afternoon": { "risk_score": 0.676, "risk_level": "high" },
    "evening":   { "risk_score": 0.724, "risk_level": "high" },
    "night":     { "risk_score": 0.785, "risk_level": "high" },
    "peak_bucket": "night"               // franja más insegura
  },

  "geometry": {
    "type": "LineString",
    "coordinates": [ [-58.4205, -34.5805], /* ... */ [-58.3813, -34.6037] ]  // [lng, lat]
  }
}
```

### Semántica de los campos base

- **`geometry`** — GeoJSON `LineString`, orden **`[lng, lat]`**. La polilínea a dibujar.
- **`risk_score` / `risk_level`** — score agregado (0..1) y su nivel (umbrales del modelo activo).
- **`max_edge_risk`** — el peor tramo puntual (coincide con `riskiest_segment.risk_score`).
- **`*_vs_fastest_*`** — comparaciones contra el `fastest` de **esta misma respuesta**. 0 en fastest.
- **`crime_metrics`** — **sumas de exposición por tramo**, NO incidentes distintos (ver caveat abajo).

### Metadata explicativa — semántica

- **`riskiest_segment`** — la cuadra de mayor `risk_score` (igual a `max_edge_risk`). Trae su
  ubicación (`point` = mediopunto del tramo) y el desglose de delitos de ESE tramo. Responde "esta
  ruta es más insegura **por esta cuadra**". Se omite solo si la ruta no tiene tramos.
- **`segments[]`** — vista mínima por cuadra, en orden: `risk_score`, `robbery_count`,
  `length_meters`, `point`. Permite comparar tramo a tramo contra una ruta casi paralela y pintar la
  polilínea por nivel de riesgo. La suma de `length_meters` ≈ `distance_meters`.
- **`dominant_factor`** — tipo de delito con mayor conteo en la ruta (enum); `none` si no hay conteos.
- **`armed_share_percent`** — proporción de incidentes con arma. Soporta "más insegura por robos con
  arma".
- **`time_of_day_risk`** — riesgo de la MISMA ruta en las 4 franjas para el `weekday_type` resuelto,
  con `peak_bucket` = la peor. Grano de franja, **no horario exacto**. Se omite si la ruta no tiene
  tramos.

> ⚠️ **Los conteos son sumas de exposición por tramo, no incidentes distintos** (un delito influye
> varias cuadras y cuenta en cada una). `dominant_factor` y `armed_share_percent` son comparaciones
> **relativas** válidas; presentarlos como exposición relativa, nunca como "N delitos en esta ruta".

### ⚠️ `routes[]` tiene longitud variable

Normalmente 4; si KSP no encuentra candidato válido, **`least_safe_candidate` se omite** → 3.
**Iterar `routes[]` por lo que llegue y buscar por `kind`** — nunca asumir índices fijos.

---

## 5. Errores

Envelope: `{ "error": "<código>", "message": "<texto>" }`.

| HTTP | `error` | Cuándo |
|------|---------|--------|
| 400  | `invalid_request` | Falta/!parsea coordenada, fuera de CABA, origen=destino, o `datetime` no RFC3339. |
| 400  | `origin_or_destination_outside_walkable_graph` | Punto a > 150 m de la red caminable. |
| 404  | `route_not_found` | Pegan al grafo pero no hay camino caminable. |
| 503  | `risk_model_unavailable` | No hay modelo de riesgo activo (problema del servidor). |
| 500  | `internal_error` | Error interno inesperado. |

---

## 6. Tipos TypeScript

```ts
export type RouteKind = "fastest" | "balanced" | "safest" | "least_safe_candidate";
export type RiskLevel = "low" | "moderate" | "high";
export type TimeBucket = "morning" | "afternoon" | "evening" | "night";
export type WeekdayType = "weekday" | "weekend";
export type DominantFactor = "robbery" | "theft" | "threats" | "none";

export interface LatLng { lat: number; lng: number; }

export interface ModelVersionInfo { id: number; name: string; type: string; train_until: string; }

export interface CrimeMetrics {
  crime_count: number; robbery_count: number; theft_count: number; threats_count: number;
  armed_count: number; motorcycle_count: number; same_bucket_crime_count: number;
}

export interface GeoJSONLineString { type: "LineString"; coordinates: [number, number][]; } // [lng, lat]

export interface RiskiestSegment {
  risk_score: number; risk_level: RiskLevel; length_meters: number; point: LatLng;
  crime_count: number; robbery_count: number; armed_count: number;
  theft_count: number; threats_count: number; motorcycle_count: number;
}

export interface RouteSegment {
  risk_score: number; robbery_count: number; length_meters: number; point: LatLng;
}

export interface BucketRisk { risk_score: number; risk_level: RiskLevel; }

export interface TimeOfDayRisk {
  morning: BucketRisk; afternoon: BucketRisk; evening: BucketRisk; night: BucketRisk;
  peak_bucket: TimeBucket;
}

export interface SafeRoute {
  kind: RouteKind;
  distance_meters: number;
  duration_minutes: number;
  risk_score: number;
  risk_level: RiskLevel;
  extra_distance_vs_fastest_meters: number;
  extra_duration_vs_fastest_minutes: number;
  risk_reduction_vs_fastest_percent: number;
  high_risk_edge_meters: number;
  high_risk_edge_percent: number;
  max_edge_risk: number;
  avg_edge_risk: number;
  crime_metrics: CrimeMetrics;
  riskiest_segment?: RiskiestSegment;   // omitido si la ruta no tiene tramos
  segments?: RouteSegment[];
  dominant_factor: DominantFactor;
  armed_share_percent: number;
  time_of_day_risk?: TimeOfDayRisk;     // omitido si la ruta no tiene tramos
  geometry: GeoJSONLineString;
}

export interface SafeRoutesResponse {
  origin: LatLng; destination: LatLng; datetime: string;
  time_bucket: TimeBucket; weekday_type: WeekdayType;
  model_version: ModelVersionInfo; routes: SafeRoute[];  // 1..4; buscar por `kind`
}

export interface ApiError { error: string; message: string; }
```

Fetch helper:

```ts
export async function fetchSafeRoutes(
  base: string, o: LatLng, d: LatLng, at?: Date,
): Promise<SafeRoutesResponse> {
  const p = new URLSearchParams({
    origin_lat: String(o.lat), origin_lng: String(o.lng),
    dest_lat: String(d.lat), dest_lng: String(d.lng),
  });
  if (at) p.set("datetime", at.toISOString()); // toISOString es RFC3339
  const res = await fetch(`${base}/api/v1/routes/safe?${p}`);
  if (!res.ok) {
    const err = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(err?.message ?? `HTTP ${res.status}`);
  }
  return res.json();
}
```

---

## 7. Renderizado del mapa

La `geometry` es GeoJSON con orden **`[lng, lat]`**:
- **Mapbox GL / MapLibre / OpenLayers / deck.gl** → nativo, pasar la `geometry` tal cual.
- **Leaflet** → usa `[lat, lng]`: envolver en `Feature` y usar `L.geoJSON`, o invertir cada par.

Wrap recomendado:

```ts
const toFeature = (r: SafeRoute) => ({
  type: "Feature" as const, geometry: r.geometry,
  properties: { kind: r.kind, risk_level: r.risk_level },
});
```

Colores sugeridos: `safest` verde · `balanced` naranja · `fastest` rojo · `least_safe_candidate`
gris punteado. Para resaltar la cuadra problemática, marcar `riskiest_segment.point`; para un
heatmap, colorear cada `segments[i].point`/tramo por `segments[i].risk_score`.

---

## 8. Recomendaciones de UX

- **Explicar el porqué con la metadata** (el backend NO manda texto; el FE lo compone). Ejemplos:
  - "Esta ruta sube a **alto** por una cuadra cerca de (`riskiest_segment.point`) con
    `riskiest_segment.robbery_count` robos, incl. `armed_count` con arma."
  - "Predomina **`dominant_factor`**; `armed_share_percent`% de los incidentes fueron con arma."
  - "Más peligrosa de **`time_of_day_risk.peak_bucket`** — considerá viajar en otra franja."
- Comparar contra `fastest` con `extra_distance_vs_fastest_meters` /
  `risk_reduction_vs_fastest_percent` ("+903 m, −50% de exposición").
- Para "por qué esta ruta es más insegura que la de al lado": usar `segments[]` (riesgo + robos por
  cuadra) y `riskiest_segment` para señalar el tramo que las diferencia.
- Tratar los conteos como **exposición relativa**, nunca como conteo de delitos absolutos.
- Mantener el disclaimer de "exposición histórica estimada, sin garantías".
- Manejar `least_safe_candidate` / campos omitidos ausentes sin romper el layout.

---

## 9. Ejemplo end-to-end (verificado)

Request: Obelisco → Plaza Italia, 23:00. `time_bucket=night`, `weekday_type=weekday`, modelo
`network_temporal_edge_risk_v1`:

| `kind`                 | distancia | duración  | `risk_score` | `risk_level` |
|------------------------|-----------|-----------|--------------|--------------|
| `fastest`              | 4998 m    | 59.5 min  | 0.939        | high         |
| `balanced`             | 5901 m    | 70.2 min  | 0.470        | moderate     |
| `safest`               | 5968 m    | 71.1 min  | 0.464        | moderate     |
| `least_safe_candidate` | 4998 m    | 59.5 min  | 0.939        | high         |

→ `balanced` reduce ~50% la exposición a cambio de +903 m / +11 min, y su `time_of_day_risk` y
`riskiest_segment` explican dónde y cuándo se concentra el riesgo.
