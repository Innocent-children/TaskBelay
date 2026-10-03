<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="packages/webui/src/assets/taskbelay-wordmark-dark.svg" />
    <img src="packages/webui/src/assets/taskbelay-wordmark.svg" width="360" height="180" alt="TaskBelay" />
  </picture>
</p>

<h1 align="center">TaskBelay</h1>

<p align="center"><strong>Programación prolongada con IA, con aseguramiento.</strong></p>

<p align="center">
  <a href="README.md">English</a> · <a href="README_zh-CN.md">简体中文</a> · <a href="README_zh-TW.md">繁體中文</a> · <a href="README_ja.md">日本語</a> · <a href="README_ko.md">한국어</a> · <a href="README_es.md">Español</a> · <a href="README_fr.md">Français</a> · <a href="README_de.md">Deutsch</a> · <a href="README_pt-BR.md">Português (Brasil)</a>
</p>

## Qué puedes hacer con TaskBelay

Tu agente de programación decide el siguiente paso técnico.<br />
TaskBelay mantiene la tarea bajo control.

Úsalo con Codex, DeepSeek, Claude Code o ZCode. El agente razona sobre el código; TaskBelay conserva el alcance, los límites de verificación, el estado y los registros de recuperación.

En escalada, quien asegura controla la cuerda mientras quien escala elige la ruta. TaskBelay aplica esa idea a la programación: el agente toma las decisiones técnicas; TaskBelay comprueba el alcance aprobado y los límites de verificación y usa los registros guardados para recuperarse de fallos o resultados inciertos. No es un entorno aislado ni otro agente de programación.

- **Alcance explícito:** Compara los cambios reales con los archivos aprobados. El trabajo fuera del plan requiere una decisión.
- **Verificación acotada:** Planifica las comprobaciones pertinentes y sus límites. Ampliarlas requiere un motivo concreto.
- **Estado persistente:** El estado de referencia de la tarea se guarda localmente entre sesiones.
- **Recuperación segura:** Consulta el estado y las operaciones guardadas para resolver fallos o resultados inciertos antes de reintentar.

Resulta útil para trabajo de repositorio que abarca varias sesiones o necesita límites claros de
archivos y pruebas. Para preguntas puntuales, explicaciones de código y pequeños cambios que no
necesitan guardar el progreso, suele ser más sencillo usar Codex, DeepSeek, Claude Code o ZCode directamente.

## Inicio rápido

> Instala Node.js `>=24` y el Host que vayas a utilizar. Consulta los requisitos de versión y las plataformas verificadas en la [matriz de soporte](docs/SUPPORT-MATRIX_en.md).

### 1. Instala TaskBelay

Instala TaskBelay y sigue las instrucciones de activación para [Codex](docs/CODEX_en.md), [DeepSeek](docs/DEEPSEEK_en.md), [Claude Code](docs/CLAUDE_en.md) o [ZCode](docs/ZCODE_en.md). El paquete está dirigido a Windows x64 y macOS arm64; la validación nativa de ZCode en macOS sigue pendiente.

```sh
npm install -g taskbelay@latest
taskbelay
```

Elige tu Host entre las opciones del instalador que estés utilizando. En Codex, revisa y autoriza el hook de TaskBelay en `/hooks`; en DeepSeek, reinicia el Profile seleccionado. En Claude Code, recarga los plugins o inicia una conversación nueva y revisa los permisos solicitados.

En ZCode, instala y activa el plugin en Settings → Plugins y abre una conversación nueva para activar los hooks. Preparar el paquete local no confirma que ZCode lo haya cargado.

### 2. Inicia una tarea

Tras completar la instalación correspondiente, envía uno de estos mensajes en la conversación del Host:

**Codex**

```text
$taskbelay-codex:taskbelay Añade un límite de frecuencia para los inicios de sesión fallidos. Modifica solo archivos de autenticación y ejecuta como máximo 4 comprobaciones dirigidas.
```

**DeepSeek Harness**

```text
/taskbelay Añade un límite de frecuencia para los inicios de sesión fallidos. Modifica solo archivos de autenticación y ejecuta como máximo 4 comprobaciones dirigidas.
```

**Claude Code**

```text
/taskbelay-claude:taskbelay Añade un límite de frecuencia para los inicios de sesión fallidos. Modifica solo archivos de autenticación y ejecuta como máximo 4 comprobaciones dirigidas.
```

**ZCode**

Selecciona `taskbelay` en el menú `/` → Skills del cuadro de entrada y describe la tarea.

```text
Usa TaskBelay para limitar los inicios de sesión fallidos. Modifica solo archivos de autenticación y ejecuta como máximo 4 comprobaciones dirigidas.
```

Envía estos mensajes en la conversación, no en una terminal. Describe el objetivo, los criterios de
aceptación, el alcance de archivos y el límite de pruebas.

La primera respuesta evalúa la solicitud y pregunta si quieres trabajar directamente o usar TaskBelay.
Al elegir TaskBelay, se crea por defecto una nueva rama de tarea desde el HEAD actual en el directorio
actual. Confirma la rama y si los cambios sin confirmar existentes forman parte de la tarea. Se conservan
las dependencias, la configuración local, los archivos y el estado del índice; la sesión continúa si puede
acceder a todos los directorios participantes.

También puedes elegir explícitamente usar la rama actual o crear un worktree de Git dedicado. Para un
worktree se seleccionan además una fuente local o remota y una rama inicial. Codex abre el nuevo
directorio cuando el entorno lo permite; DeepSeek y Claude Code proporcionan el comando de reinicio correspondiente.

Cada directorio admite una sola Task activa. También se observan las ediciones manuales o de otras
herramientas, y cambiar de rama durante la tarea pausa el proceso. Los directorios y ramas locales se
conservan al terminar; los cambios sin confirmar deben tenerse en cuenta al iniciar la siguiente tarea.

Antes de implementar, revisa y comenta los requisitos, el diseño, las tareas, los archivos previstos y el plan de verificación. El desarrollo comienza tras tu aprobación explícita del plan completo. Los cambios de plan o la ampliación del alcance de archivos requieren una nueva aprobación. Elegir TaskBelay o un worktree no sustituye esa aprobación.

Para corregir el nombre de la rama de una Task activa, pide al Host que prepare el cambio en TaskBelay.
Después, cambia solo el nombre de la rama de un repositorio, conservando el directorio, HEAD, el índice
y los archivos. TaskBelay comprueba el resultado y continúa la misma Task. Solo se puede cancelar antes
de que cambie el estado original. En Codex puedes sustituir explícitamente un inicio aún no ejecutado;
si falló, hacen falta pruebas de que no se llamó a ninguna operación de destino. Inspecciona primero
los resultados inciertos o las operaciones parciales.

Las revisiones sucesivas del plan conservan todas las referencias guardadas dentro de la Task, sin una
capa de archivo ni un máximo de entradas configurado. Siguen aplicándose los límites de recursos y del
rango de los números de revisión. Las respuestas muestran una primera página acotada y el total; pide
al Host que lea el historial completo por páginas para revisar decisiones anteriores. Consulta las
[operaciones del espacio de trabajo](docs/WORKTREE-SOURCES_en.md) y la
[referencia de comandos](docs/COMMANDS_en.md) para conocer los pasos y límites.

### 3. Retoma y consulta el progreso

Después de reiniciar la sesión, vuelve al directorio original y solicita continuar la tarea. TaskBelay
retoma el progreso guardado. Si ese directorio desapareció o fue reemplazado, la tarea se pausa hasta
que lo restaures o abandones explícitamente la tarea.

En DeepSeek Harness, incluye `/taskbelay` en el mensaje que solicita retomar la tarea.

En Claude, vuelve al directorio y a la conversación originales e indica con `/taskbelay-claude:taskbelay` que deseas continuar la tarea guardada.

En ZCode, abre el directorio original, selecciona la Skill TaskBelay y pide continuar la tarea guardada. Si se prepara otro directorio, sigue las instrucciones recibidas para abrir ese espacio de trabajo.

Estos comandos usan el gestor global instalado. Para instalaciones desde el código fuente, utiliza la entrada correspondiente de la guía.

```bash
# Consultar las integraciones instaladas
taskbelay status --host all

# Abrir la vista local de tareas
taskbelay webui start
```

Para instalación no interactiva, Profiles de DSH personalizados, actualizaciones, reparación y
eliminación, consulta la [referencia de comandos](docs/COMMANDS_en.md).

## Mascota de escritorio

La mascota requiere un Adapter configurado y la aplicación de escritorio instalada. Instalar solo un Adapter no instala la aplicación de escritorio.

La mascota muestra varias tareas en burbujas superpuestas y abre la WebUI de cada una. Prioriza las tareas bloqueadas y pasa automáticamente a otra tarea pendiente cuando termina la actual; también puedes fijar una tarea. Puedes personalizar su apariencia, controlar animaciones, cambiar su tamaño e iniciarla o detenerla por separado.

```bash
taskbelay pet start
taskbelay pet stop
```

Las aplicaciones de escritorio están destinadas a macOS arm64 y Windows 10/11 x64. Consulta la
[guía de la mascota](docs/DESKTOP-PETS_en.md) para instalarla y manejarla, y la
[matriz de soporte](docs/SUPPORT-MATRIX_en.md) para conocer la disponibilidad verificada.

## Límites de uso

TaskBelay controla el flujo de la tarea, no los permisos del sistema operativo. No intercepta cada operación de archivos o comando de shell.

Un worktree dedicado separa los cambios de código. Los procesos, el acceso a la red, las credenciales
y los servicios externos siguen compartidos con tu entorno.

Completar una tarea no crea commits, envía cambios ni elimina su worktree automáticamente. Esas
operaciones requieren tu autorización por separado.

## Documentación

- **Uso:** [Codex](docs/CODEX_en.md) · [DeepSeek](docs/DEEPSEEK_en.md) · [Claude Code](docs/CLAUDE_en.md) · [ZCode](docs/ZCODE_en.md) · [Comandos](docs/COMMANDS_en.md) · [Control Center](docs/WEBUI_en.md)
- **Proyecto:** [Definición del producto](docs/PRODUCT_en.md) · [Matriz de soporte](docs/SUPPORT-MATRIX_en.md) · [Seguridad](SECURITY.md)
- **Desarrollo y contribuciones:** [Índice de documentación](MANIFEST_en.md) · [Guía de contribución](CONTRIBUTING.md)

## Comunidad

[LINUX DO](https://linux.do/)

## Licencia

[Apache License 2.0](LICENSE)
