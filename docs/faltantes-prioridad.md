# Implementaciones BPMN Faltantes por Prioridad

🌐 [English](missing-priority.md) | **Español**

## 🔴 Alta Prioridad

| # | Elemento | Impacto |
|---|----------|---------|
| 1 | **Autenticación/Autorización** con middleware JWT | Seguridad en la API |

## 🟡 Prioridad Media

| # | Elemento | Impacto |
|---|----------|---------|
| 5 | **Business Rule Task** (DMN engine) | Emulable con ScriptTask pero sin motor de reglas dedicado |
| 6 | **Send Task / Receive Task** como elementos dedicados | Emulable con MessageThrow/Catch pero sin semántica explícita |
| 7 | **Data Objects / Data Stores** con persistencia formal | Variables planas usadas en su lugar — sin semántica de datos formal |
| 8 | **Pools / Lanes** para colaboración entre participantes | Sin colaboración entre procesos |
| 9 | **Message Flow** entre pools | Sin comunicación entre participantes |
| 10 | **Formal Expressions** (FEEL) | Usa govaluate como alternativa pragmática |
| 11 | **Data Associations** (input/output specifications) | Mapeo implícito vía variables |

## 🟢 Prioridad Baja

| # | Elemento | Impacto |
|---|----------|---------|
| 12 | **Escalation Event** | Sin escalamiento a rol superior — caso específico |
| 13 | **Compensation Event** | Sin rollback parcial — caso específico |
| 14 | **Conditional Event** | Sin eventos basados en cambios de estado — nicho |
| 15 | **Link Event** | Conector entre diagramas — no crítico |
| 16 | **Complex Gateway** | Lógica de merging compleja — muy específico, poca demanda |
| 17 | **Manual Task** | Emulable con UserTask |
| 18 | **Transaction** sub-process | Sin sub-procesos, no hay transacciones |
| 19 | **Resource Assignment** formal (resourceRole) | Asignación simplificada vía campos |
| 20 | **Rendering de diagramas** BPMN a PNG (gg/fogleman) | Dependencia presente, solo metadatos |
| 21 | **gRPC** API | Planeado pero no implementado |

## Resumen

| Prioridad | Cantidad |
|-----------|----------|
| 🔴 Alta | 1 |
| 🟡 Media | 7 |
| 🟢 Baja | 10 |
| **Total** | **18** |

> Los 4 items de alta prioridad representan las mayores limitaciones para uso en producción.
> Los 7 de prioridad media cubren casos de uso comunes pero con workarounds disponibles.
> Los 10 de baja prioridad son casos específicos o features emulables.
