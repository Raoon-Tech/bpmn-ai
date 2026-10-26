# Referencia de la CLI

🌐 [English](cli.md) | **Español**

## Uso general

```bash
bpmn-ai [comando] [flags]
```

Sin comando, arranca el servidor (`serve`). Las flags pueden ir antes o después de los argumentos posicionales.

## Comandos

### serve

```bash
bpmn-ai serve
```

Arranca el motor HTTP. Aplica migraciones pendientes si `DB_AUTO_MIGRATE=true`.

### version

```bash
bpmn-ai version [--json]
```

Muestra versión, commit y fecha de compilación.

### doctor

```bash
bpmn-ai doctor
```

Comprueba configuración, conexión a la BD, migraciones pendientes, directorio de auditoría, configuración de IA y disponibilidad del puerto HTTP.

### config show

```bash
bpmn-ai config show [--json]
```

Muestra la configuración efectiva con los secretos enmascarados.

### migrate

Ver [Migraciones](migraciones.md).

### db

```bash
bpmn-ai db ping
bpmn-ai db stats
bpmn-ai db seed <archivo.bpmn>
```

- `ping`: comprueba la conexión y muestra la versión del motor de BD.
- `stats`: cuenta filas por tabla.
- `seed`: despliega un archivo BPMN en la base de datos.

### process

- `process list`: lista las definiciones de proceso desplegadas (id, nombre, versión, fecha).
- `process get <id>`: muestra metadatos y número de elementos/flujos.
- `process deploy <archivo.bpmn>`: parsea y guarda un archivo BPMN (equivalente a `db seed`).
- `process delete <id> [--force]`: elimina una definición. Destructivo: requiere `--force` o `DB_ALLOW_DESTRUCTIVE=true`.

```bash
bpmn-ai process list
bpmn-ai process get 96870a21-8414-4d9d-9de6-67cf735bbc6e
bpmn-ai process deploy examples/approval-process.bpmn
bpmn-ai process delete 96870a21-... --force
```

### case

- `case list [--status STATUS] [--limit N]`: lista instancias, filtrables por estado (`CREATED`, `IN_PROGRESS`, `WAITING`, `COMPLETED`, `ERROR`, `TERMINATED`).
- `case get <id>`: muestra estado, variables y fechas.
- `case start <processID> [--title T] [--var k=v]... [--timeout 30s]`: crea un caso y ejecuta el motor de forma **sincrónica** hasta completar o esperar; muestra el estado final y las tareas pendientes.
- `case history <id>`: muestra el log de ejecución del caso.
- `case cancel <id>`: marca el caso como `TERMINATED`.

```bash
bpmn-ai case start 96870a21-... --var approved=true --title "Pedido 42"
bpmn-ai case list --status WAITING
bpmn-ai case get 04a1df0f-551d-4e08-9a21-00c359c86752
bpmn-ai case history 04a1df0f-...
bpmn-ai case cancel 04a1df0f-...
```

### task

- `task list <caseID>`: lista las tareas de usuario activas del caso (flow id, elemento, hilo, inicio).
- `task claim <id> [--user USER]`: reclama una tarea para un usuario.
- `task complete <id> [--var k=v]... [--timeout 30s]`: completa la tarea, actualiza el flujo y **reanuda el caso** de forma sincrónica.

```bash
bpmn-ai task list 04a1df0f-...
bpmn-ai task claim dbb944cb-... --user ana
bpmn-ai task complete dbb944cb-... --var approved=true
```

### job

- `job list [--status STATUS] [--limit N]`: lista jobs pendientes (solo `PENDING` es consultable).
- `job retry <id>`: reencola un job (estado `PENDING`, contador reiniciado, programado ahora).
- `job drain [--limit N] [--timeout 5m]`: procesa hasta `N` jobs pendientes de forma **sincrónica en primer plano** usando el handler del motor, hasta vaciar la cola o agotar el timeout. Útil para cron, CI o depuración sin worker en segundo plano.

```bash
bpmn-ai job list
bpmn-ai job retry 1234abcd-...
bpmn-ai job drain --limit 50
```

### message

En BPMN, un evento **MessageCatch** (captura intermedia o boundary de mensaje) deja la instancia en `WAITING` hasta que llega un mensaje con esa referencia. `message send` entrega el mensaje a una instancia concreta y la reanuda.

```bash
bpmn-ai message send 04a1df0f-... order-confirmed --var order_id=123
```

Útil para integrar sistemas externos, probar correlación de mensajes o completar casos manualmente sin la API HTTP.

### signal

Un **SignalThrow** difunde una señal a **todas** las instancias que tengan un **SignalCatch** en espera para esa referencia (broadcast global, no ligado a una instancia). Devuelve los ids de las instancias afectadas.

```bash
bpmn-ai signal send customer-updated --var tier=premium
```

Útil para notificaciones globales (cambios de estado compartido, eventos de dominio) y para probar escenarios de broadcast.

### ai

Gestiona la configuración multi-proveedor de IA almacenada en la tabla `ai_providers`.

- `ai provider list`: lista los proveedores configurados (claves enmascaradas).
- `ai provider add <name> --type openai|anthropic --key <key> [--base-url URL] [--model MODEL] [--default] [--key-stdin]`: añade o actualiza un proveedor. Usa `--key-stdin` para leer la clave desde stdin y no dejarla en el historial del shell.
- `ai provider remove <name>`: elimina un proveedor.
- `ai provider show <name> [--reveal]`: muestra un proveedor (clave enmascarada salvo `--reveal`).
- `ai provider set-default <name>`: marca un proveedor como predeterminado.

```bash
bpmn-ai ai provider add openrouter --type openai --key sk-or-... --base-url https://openrouter.ai/api/v1 --default
echo sk-ant-... | bpmn-ai ai provider add anthropic --type anthropic --key-stdin
bpmn-ai ai provider list
bpmn-ai ai provider show openrouter
bpmn-ai ai provider set-default anthropic
bpmn-ai ai provider remove openrouter
```

Las claves se guardan en texto claro en la base de datos; restringe el acceso a la BD en consecuencia. Los proveedores cargados desde la BD se añaden al router de modelos al arrancar (las variables de entorno tienen prioridad para el gateway primario).

### ai profile

Gestiona los perfiles de enrutado de modelos (`complex`, `fast`, `cheap`, `auto`) almacenados en la tabla `ai_model_profiles`.

```bash
bpmn-ai ai profile list
bpmn-ai ai profile get <name>
bpmn-ai ai profile set <name> --model <model> [--max-tokens N] [--priority quality|speed|cost]
bpmn-ai ai profile reset [--name <name>]
```

Los perfiles almacenados en la BD sobrescriben los defaults integrados al arrancar. `reset` sin `--name` elimina todos los perfiles almacenados (se aplican los defaults integrados).

## Códigos de salida

- `0`: éxito
- `1`: error de ejecución
- `2`: uso incorrecto
