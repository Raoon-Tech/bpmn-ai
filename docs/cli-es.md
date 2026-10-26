# Referencia de la CLI

🌐 [English](cli.md) | **Español**

## Uso general

```bash
bpmn-ai [comando] [flags]
```

Sin comando, arranca el servidor (`serve`).

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

### doctor

```bash
bpmn-ai doctor
```

Comprueba configuración, base de datos, migraciones pendientes, auditoría, IA y puerto HTTP.

### config show

```bash
bpmn-ai config show [--json]
```

Muestra la configuración efectiva con secretos enmascarados.

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

## Códigos de salida

- `0`: éxito
- `1`: error de ejecución
- `2`: uso incorrecto
