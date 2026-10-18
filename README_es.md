# TaskBelay

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja.md) · [한국어](README_ko.md) · [Español](README_es.md) · [Français](README_fr.md) · [Deutsch](README_de.md) · [Português](README_pt-BR.md)

**Control de ingeniería para tareas reales con agentes de programación.**

TaskBelay acompaña a Codex, DeepSeek Harness, Claude Code y ZCode. Conserva juntos el objetivo autorizado, los cambios reales y las pruebas de verificación para continuar entre sesiones con menos decisiones repetidas. El agente sigue siendo responsable del código y del criterio técnico.

## Qué permite

- Mantener objetivos, exclusiones y restricciones de interfaces, datos, dependencias y diseño durante el desarrollo. Revisar cambios dentro de archivos, además de sus rutas. Corregir desviaciones dentro de la autorización existente y consultar al usuario cuando deba cambiar la autorización o una decisión importante.
- Ejecutar comprobaciones locales autorizadas y guardar códigos de salida reales, entradas, condiciones del entorno y resultados. Las declaraciones, la ejecución y las conclusiones de aceptación se distinguen. Los cambios relevantes invalidan pruebas; las comprobaciones independientes con un alcance justificado pueden conservarse. Replanificar no reinicia el coste acumulado.
- Alternar implementación y verificación, revisar requisitos y recuperar resultados, intentos fallidos y ejecuciones inciertas. El historial reciente tiene límites; los resultados vigentes y los costes acumulados se conservan.

Las comprobaciones de comandos usan `taskbelay check run` desde la herramienta nativa de procesos del host, con sus permisos y entorno. La WebUI permite buscar tareas, filtrar por host y estado, elegir idioma, responder decisiones y recuperar ejecuciones. Cada objetivo requiere una comprobación de aceptación aprobada; aceptar fallos de regresión existentes no la sustituye. Leer el estado guardado no escribe datos de consumo.

Las dudas técnicas conservan su identidad hasta resolverse; omitir el texto no las elimina. Las limitaciones de la entrega se derivan de esas decisiones guardadas. El uso del Host distingue entre no informado, parcial e informado; los valores no disponibles siguen siendo desconocidos. La ayuda de cada método incluye ejemplos de entrada y salida y cómo tratar errores, sin consultar el código fuente. Las nuevas ramas de Task no heredan un upstream implícito.

## Empezar

Se requiere Node.js >=24 y un agente instalado. El desarrollo desde código fuente también necesita Go >=1.26 y pnpm >=11 <12. Este código incluye una evolución incompatible del contrato de ingeniería; los paquetes publicados y los registros de soporte estable no han cambiado. Compile este código para usar el nuevo contrato. Conserve los datos anteriores y elija explícitamente otro directorio de datos. No hay migración ni reinicio automático.

Instalación pública:

```sh
npm install -g taskbelay@latest
taskbelay
```

Compilar e instalar el código actual localmente:

```sh
pnpm install --frozen-lockfile --ignore-scripts
node scripts/taskbelay-local.mjs install --host codex --yes
```

Active el adaptador con su [guía](packages/codex/README.md) y solicite un resultado acotado:

```text
Usa TaskBelay para limitar los intentos fallidos de inicio de sesión. Conserva la API pública de autenticación y los datos existentes; no rediseñes la gestión de cuentas. Ejecuta las pruebas existentes pertinentes e informa de los resultados reales.
```

Indique el resultado y las restricciones necesarias. El agente explica su enfoque y trabaja con la autorización existente. Los ajustes habituales de implementación y comprobación no requieren otra aprobación; cambiar objetivos o restricciones explícitas sí requiere su decisión real. Los resultados válidos se reutilizan. El consumo se registra continuamente y los límites solo se aplican cuando usted los establece. La WebUI combina una lista compacta de tareas con las pestañas Resumen, Registros de comprobación y Requisitos y consumo. La salida se despliega bajo su comprobación; mantiene las respuestas, la recuperación y el archivado. Actualizar registros lee el contenido guardado; Comprobar código actual vuelve a observar las condiciones reales. Reanude con el Task ID guardado en el espacio original.

El agente muestra el directorio, la rama y los cambios existentes y utiliza el espacio elegido o claramente indicado por el contexto. Solo pregunta si hay ambigüedad, conflictos o una operación que necesita un nuevo espacio. Crear ramas o worktrees e integrar cambios del repositorio de origen son operaciones opcionales y siguen la autorización real.

```sh
taskbelay webui start
```

## Límites

Los hooks estructurados cubren rutas compatibles; los cambios por shell y herramientas externas se observan en puntos de control. Solo las comprobaciones ejecutadas por Core tienen procedencia de ejecución Core. Las entradas y condiciones declaradas no incluyen automáticamente dependencias ignored, bases de datos o servicios remotos. El razonamiento semántico es explícito, pero no demuestra corrección. TaskBelay no es un sandbox ni otro agente de programación.

La carga de Core se mide; el análisis, la espera y los tokens del agente son valores declarados y pueden ser incompletos. Las tareas mecánicas pequeñas suelen ser más simples con el agente nativo. Las pruebas del código muestran capacidades, no una reducción demostrada de defectos o supervisión humana. Finalizar conserva archivos y no autoriza Git, publicación ni borrado.

## Más información

[Product](docs/PRODUCT_en.md) · [Commands](docs/COMMANDS_en.md) · [Support and verification](docs/SUPPORT-MATRIX_en.md) · [Codex](packages/codex/README.md) · [DeepSeek](packages/deepseek/README.md) · [Claude Code](docs/CLAUDE_en.md) · [ZCode](docs/ZCODE_en.md)
