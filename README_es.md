# TaskBelay

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja.md) · [한국어](README_ko.md) · [Español](README_es.md) · [Français](README_fr.md) · [Deutsch](README_de.md) · [Português](README_pt-BR.md)

**Control de ingeniería para tareas reales con agentes de programación.**

TaskBelay acompaña a Codex, DeepSeek Harness, Claude Code y ZCode. Conserva juntos el objetivo autorizado, los cambios reales y las pruebas de verificación para continuar entre sesiones con menos decisiones repetidas. El agente sigue siendo responsable del código y del criterio técnico.

## Qué permite

- Mantener objetivos, exclusiones y restricciones de interfaces, datos, dependencias y diseño durante el desarrollo. Revisar cambios dentro de archivos, además de sus rutas. Corregir desviaciones dentro de la autorización existente y consultar al usuario cuando deba cambiar la autorización o una decisión importante.
- Ejecutar comprobaciones locales autorizadas y guardar códigos de salida reales, entradas, condiciones del entorno y resultados. Las declaraciones, la ejecución y las conclusiones de aceptación se distinguen. Los cambios relevantes invalidan pruebas; las comprobaciones independientes con un alcance justificado pueden conservarse. Replanificar no reinicia el coste acumulado.
- Alternar implementación y verificación, revisar requisitos y recuperar resultados, intentos fallidos y ejecuciones inciertas. El historial reciente tiene límites; los resultados vigentes y los costes acumulados se conservan.

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

Indique el resultado y los límites. Antes de implementar, el agente presenta su comprensión, causa raíz, plan completo, responsabilidades, alcance, comprobaciones y límites de coste para su confirmación explícita. Los cambios importantes requieren otra confirmación; el progreso normal conserva la aprobación válida. Cada comprobación debe cubrir una carencia actual y los resultados válidos se reutilizan. La WebUI muestra el plan, las pruebas y el coste, permite confirmar y archivar tareas terminadas y abre tareas mediante enlaces directos. Reanude con el Task ID guardado en su espacio original.

```sh
taskbelay webui start
```

## Límites

Los hooks estructurados cubren rutas compatibles; los cambios por shell y herramientas externas se observan en puntos de control. Solo las comprobaciones ejecutadas por Core tienen procedencia de ejecución Core. Las entradas y condiciones declaradas no incluyen automáticamente dependencias ignored, bases de datos o servicios remotos. El razonamiento semántico es explícito, pero no demuestra corrección. TaskBelay no es un sandbox ni otro agente de programación.

La carga de Core se mide; el análisis, la espera y los tokens del agente son valores declarados y pueden ser incompletos. Las tareas mecánicas pequeñas suelen ser más simples con el agente nativo. Las pruebas del código muestran capacidades, no una reducción demostrada de defectos o supervisión humana. Finalizar conserva archivos y no autoriza Git, publicación ni borrado.

## Más información

[Product](docs/PRODUCT_en.md) · [Commands](docs/COMMANDS_en.md) · [Support and verification](docs/SUPPORT-MATRIX_en.md) · [Codex](packages/codex/README.md) · [DeepSeek](packages/deepseek/README.md) · [Claude Code](docs/CLAUDE_en.md) · [ZCode](docs/ZCODE_en.md)
