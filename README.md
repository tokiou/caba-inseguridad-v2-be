# CABA Rutas Seguras

Backend para calcular rutas caminables en la Ciudad Autónoma de Buenos Aires
priorizando una menor exposición histórica estimada a zonas con incidentes.
El sistema combina datos abiertos de delitos, un grafo peatonal de
OpenStreetMap y un modelo offline de riesgo temporal sobre PostgreSQL,
PostGIS y pgRouting.

> El producto estima exposición histórica. No ofrece garantías de seguridad ni
> reemplaza el criterio de la persona que recorre la ruta.

## Qué resuelve

- Consulta de incidentes cercanos a una coordenada mediante PostGIS.
- Rutas peatonales sobre un grafo real de calles.
- Alternativas de ruta ponderadas por riesgo histórico, horario y tipo de día.
- Explicación de la ruta: métricas, nivel de riesgo y factores relevantes.
- Usuarios, login, JWT de corta duración y refresh tokens rotatorios.
- Cache distribuida de rutas y rate limiting por endpoint, ambos opcionales.
- Pipeline ETL reproducible para normalizar y cargar los datos.

## Por qué es más escalable que un backend típico

La escalabilidad aquí se pensó como capacidad de crecer sin convertir cada
incremento de tráfico o de complejidad en un cambio transversal.

### 1. Capas con responsabilidades claras

El flujo es:

```text
HTTP -> router chi -> handler -> service -> repository -> PostgreSQL/PostGIS
```

Los handlers sólo traducen HTTP, los services contienen reglas de negocio y
los repositories encapsulan el acceso a datos. Esto permite:

- probar la lógica sin levantar una base de datos;
- cambiar la implementación de persistencia sin reescribir HTTP;
- agregar endpoints o dominios sin crear dependencias circulares;
- escalar cada tipo de trabajo de forma independiente más adelante.

### 2. La geolocalización está en la base adecuada

PostGIS resuelve proximidad, geometrías e índices espaciales en PostgreSQL, en
lugar de traer grandes volúmenes a Go para calcular distancias. pgRouting
resuelve el camino sobre el grafo de calles. Esto reduce trabajo en la API y
permite que el motor de datos use índices, planes de consulta y pools de
conexiones.

Las consultas geoespaciales usan `pgx` y SQL explícito. El CRUD relacional de
autenticación usa `sqlc`. La separación evita forzar a una herramienta a
entender funciones específicas de PostGIS que no soporta.

### 3. El cálculo pesado de riesgo no ocurre en cada request

El pipeline Network Temporal KDE corre offline: normaliza delitos, los asocia
al grafo, construye vecindades y calcula scores por contexto temporal. La API
lee los scores ya preparados. Así, el costo de procesar todo el histórico no
se multiplica por cada usuario que solicita una ruta.

Los modelos se versionan en la base y una modificación del modelo también
invalida naturalmente las claves de cache de rutas.

### 4. Redis desacopla presión y tiene degradación segura

Redis se usa para dos funciones que pueden crecer con el tráfico:

- rate limiting distribuido por endpoint e IP;
- cache de respuestas de `/api/v1/routes/safe`.

Ambas funciones son opt-in. La cache tiene TTL de una hora y es fail-open: si
Redis falla, la request intenta calcular la ruta en lugar de convertir una
dependencia auxiliar en una caída total. La API también puede funcionar sin
Redis para el modo base.

### 5. Protección específica para operaciones costosas

Cada endpoint tiene su propio límite, evitando que un consumidor de consultas
baratas consuma la cuota de rutas. Los límites actuales son:

| Endpoint | Límite |
| --- | ---: |
| `GET /api/v1/routes/safe` | 10 por minuto |
| `POST /api/v1/auth/login` | 5 por minuto |
| `GET /api/v1/crimes/nearby` | 30 por minuto |
| `GET /api/v1/roadgraph/stats` | 60 por minuto |

### 6. Pooling, observabilidad y pruebas de carga

La API usa un pool de conexiones `pgxpool`, request IDs y logs estructurados.
El endpoint de estadísticas, protegido y limitado a loopback, expone estado
del pool, runtime y contadores de cache para medir saturación.

El directorio `bench/` contiene escenarios k6 para comparar baseline, Redis,
cache, rate limiting, autenticación y agotamiento del pool. Esto permite
validar decisiones con datos en vez de asumir que una optimización funciona.

### 7. Seguridad y operación considerada desde el diseño

- passwords con bcrypt;
- access tokens JWT de vida corta;
- refresh tokens opacos almacenados como hash;
- refresh cookie HttpOnly y configuración de `Secure`/`SameSite`;
- rechazo de secretos JWT débiles fuera de desarrollo;
- contenedor multi-stage, estático, distroless y non-root;
- CI con build, vet, cobertura, race detector y chequeos de goroutines.

## Arquitectura

```text
cmd/api
  └── wiring de configuración, Postgres, Redis y router

internal/
  app/             composición y registro de rutas
  auth/            cuentas, tokens y middleware de autenticación
  crimes/          incidentes cercanos
  saferoutes/      rutas ponderadas por riesgo y cache
  roadgraph/       estado y rutas del grafo peatonal
  ratelimit/       límites distribuidos por endpoint
  observability/   estadísticas operativas de desarrollo/benchmark
  platform/        clientes de PostgreSQL y Redis
  httpx/           respuestas JSON compartidas

etl/
  python/           normalización y carga de delitos
  risk_network_kde/ cálculo offline de riesgo sobre la red

scripts/osm/       importación y limpieza offline del grafo OSM
migrations/        esquema PostGIS, grafo, riesgo y autenticación
bench/              escenarios k6 y snapshots de rendimiento
```

## API principal

Base URL local: `http://localhost:8080`

| Método | Endpoint | Descripción | Auth |
| --- | --- | --- | --- |
| `GET` | `/api/v1/health` | health check | No |
| `GET` | `/api/v1/crimes/nearby` | incidentes cercanos | No |
| `GET` | `/api/v1/roadgraph/stats` | estadísticas del grafo | No |
| `GET` | `/api/v1/roadgraph/route` | ruta sobre el grafo | No |
| `GET` | `/api/v1/routes` | ruta convencional | No |
| `GET` | `/api/v1/routes/safe` | alternativas ponderadas por riesgo | Sí |
| `POST` | `/api/v1/auth/register` | registrar usuario | No |
| `POST` | `/api/v1/auth/login` | iniciar sesión | No |
| `POST` | `/api/v1/auth/refresh` | renovar access token | Cookie |
| `POST` | `/api/v1/auth/logout` | revocar refresh token | Cookie |

La especificación OpenAPI está en `openapi.yaml` y la interfaz Swagger está
disponible en `/docs/` cuando la API está corriendo.

## Requisitos

- Go 1.25 o superior.
- Docker y Docker Compose para PostgreSQL/PostGIS, pgRouting y Redis.
- Python 3 para el ETL.
- k6 sólo para benchmarks.

## Inicio rápido

```bash
cp .env.example .env
docker compose up -d postgres redis
go run ./cmd/api
```

En otra terminal:

```bash
curl http://localhost:8080/api/v1/health
```

La base necesita tener aplicadas las migraciones y los datos cargados para
usar consultas de delitos o routing. La API puede iniciarse sin Redis si se
desactivan `REDIS_ENABLED`, `RATE_LIMIT_ENABLED` y `ROUTE_CACHE_ENABLED`.

## Desarrollo y validación

```bash
make build              # compilar todos los paquetes
make vet                # análisis estático de Go
make test               # tests unitarios
make test-race          # race detector + chequeos de goroutines
make cover              # cobertura y coverage.out
make test-integration   # requiere PostGIS, grafo y Redis poblados
```

Para cargar delitos:

```bash
cd etl/python
pip install -r requirements.txt
python analyze_raw_data.py
python normalize_crimes.py
python load_to_postgres.py
```

La construcción del grafo peatonal está documentada en los scripts de
`scripts/osm/`. Los datos fuente grandes bajo `data/raw/` y los artefactos
generados no forman parte del flujo normal de versionado.

## Configuración de escalabilidad

Las flags se pueden combinar sin cambiar código:

| Modo | Redis | Rate limit | Cache |
| --- | --- | --- | --- |
| Baseline | No | No | No |
| Redis-only | Sí | No | No |
| Rate limit | Sí | Sí | No |
| Cache | Sí | No | Sí |
| Cache + rate limit | Sí | Sí | Sí |

Variables importantes:

```env
DATABASE_URL=postgres://postgres:postgres@localhost:5434/caba_routes?sslmode=disable
REDIS_ENABLED=true
RATE_LIMIT_ENABLED=true
ROUTE_CACHE_ENABLED=true
METRICS_ENABLED=false
```

`RATE_LIMIT_ENABLED` y `ROUTE_CACHE_ENABLED` requieren
`REDIS_ENABLED=true`. `METRICS_ENABLED` debe quedar apagado salvo para
desarrollo o benchmarks, porque expone información operativa y sólo acepta
acceso loopback.

## Límites actuales

El diseño está preparado para escalar horizontalmente la API, pero el
despliegue actual no incluye todavía un orquestador, réplicas de lectura,
particionado de datos ni observabilidad externa. También quedan fuera del
alcance actual los roles de autorización, los puntos a evitar, reportes de la
comunidad y modelos ML de riesgo.

La siguiente evolución natural sería medir con los escenarios de `bench/`,
identificar el cuello de botella dominante y recién entonces decidir entre
réplicas de PostgreSQL, más instancias de API, tuning del pool o mejoras del
cache.

## Documentación relacionada

- `CLAUDE.md`: contexto técnico completo y convenciones del repositorio.
- `openspec/`: especificaciones y decisiones de cambios.
- `bench/README.md`: metodología y escenarios de benchmark.
- `docs/api/safe-routes-frontend-integration.md`: integración del endpoint de
  rutas seguras.
