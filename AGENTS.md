# Arquitectura de Agentes - Maximus CLI

## 1. Visión General del Proyecto
**Maximus CLI** (`maximus-cli`) es una herramienta de línea de comandos diseñada para agrupar, automatizar y gestionar funcionalidades de uso diario que requieren múltiples pasos o la ejecución de diversos scripts. 

* **Stack Tecnológico:** Golang, Bubbletea (TUI), SQLite3.
* **Gestión de Estado:** Base de datos local ubicada en `~/.config/maximus-cli/`.
* **Orquestación y LLMs:** Utiliza el framework **Antigravity** para alternar dinámicamente entre los modelos **Gemini Pro** (para tareas de razonamiento complejo y arquitectura) y **Gemini Flash** (para generación rápida de código y ejecución de herramientas).
* **Construcción:** Gestionado a través de un `Makefile` predefinido. No requiere configuración de variables de entorno adicionales.

---

## 2. Definición de los Agentes
Dado que el objetivo es mantener y escalar código Golang eficiente para una TUI, el sistema se divide en los siguientes agentes virtuales:

### 🤖 Agente 1: El Arquitecto (TUI & State Manager)
* **Rol:** Diseñador de interfaces terminales y gestor del estado.
* **Objetivo:** Planificar la estructura de los modelos de Bubbletea (funciones `Init`, `Update`, `View`) y diseñar los esquemas de la base de datos SQLite3.
* **Modelo preferido:** Gemini Pro (requiere mayor contexto y razonamiento).
* **Responsabilidades:**
  * Diseñar la experiencia de usuario en la terminal.
  * Estructurar consultas SQL eficientes.
  * Mantener la coherencia del estado en `~/.config/maximus-cli`.

### ⚡ Agente 2: El Ingeniero Go (Code Generator)
* **Rol:** Desarrollador backend y ejecutor de scripts.
* **Objetivo:** Escribir código Golang idiomático, eficiente y concurrente (usando *goroutines* y *channels* cuando sea necesario para no bloquear la UI de Bubbletea).
* **Modelo preferido:** Gemini Flash (optimizado para velocidad en la generación de código y tareas iterativas).
* **Responsabilidades:**
  * Implementar la lógica de negocio y la integración de scripts externos.
  * Escribir el código Go propuesto por el Arquitecto.
  * Asegurar un manejo de errores robusto en Go.

### 🛠️ Agente 3: El Revisor (Build & QA)
* **Rol:** Control de calidad y compilación.
* **Objetivo:** Asegurar que el código compila correctamente y cumple con los estándares de Go.
* **Modelo preferido:** Gemini Flash.
* **Responsabilidades:**
  * Ejecutar el `Makefile`.
  * Leer y solucionar errores de compilación o del linter (`golangci-lint` si aplica).
  * Verificar que el binario resultante es funcional.

---

## 3. Herramientas y Capacidades (Tools)
Los agentes tienen acceso a un conjunto de herramientas compartidas (pool común) orientadas al desarrollo local:

* **File System Operations:** Capacidad para leer, escribir y modificar archivos `.go`, `.sql` o scripts dentro del directorio del proyecto.
* **Terminal/Shell Execution:**
  * Ejecución de comandos Go (`go mod tidy`, `go fmt`, `go vet`).
  * Ejecución de procesos de compilación (`make build`, `make clean`).
  * Ejecución de scripts bash/sh locales de prueba.
* **SQLite Inspector:** Herramienta para leer el esquema actual de `~/.config/maximus-cli` o ejecutar consultas de validación en tiempo de desarrollo.

---

## 4. Flujo de Trabajo (Workflow)
El sistema opera bajo un modelo de **delegación jerárquica y secuencial** guiado por el usuario:

1. **Petición del Usuario:** El usuario solicita una nueva funcionalidad (ej. *"Añade un comando para limpiar contenedores de Docker inactivos"*).
2. **Planificación (Arquitecto):** El Arquitecto evalúa cómo integrar esto en la UI de Bubbletea y si requiere guardar algún estado en SQLite. Genera un plan de acción.
3. **Implementación (Ingeniero):** El Ingeniero Go toma el plan y genera/modifica los archivos `.go` necesarios utilizando sus herramientas de sistema de archivos.
4. **Validación (Revisor):** El Revisor ejecuta el `Makefile` para compilar. Si hay fallos, se retroalimenta al Ingeniero para que corrija el código. Si compila con éxito, la tarea se da por concluida.

## 5. Reglas de Operación y Seguridad (Guardrails)

Para garantizar la integridad del código y la seguridad del usuario, todos los agentes deben adherirse estrictamente a las siguientes reglas durante su ejecución:

### 🛡️ Seguridad y Privacidad (Zero Secrets Policy)
* **Prohibición de Exposición:** Los agentes **nunca** deben incluir, registrar (log), ni imprimir en consola ningún tipo de secreto, token, contraseña, clave API o información sensible del usuario.
* **Revisión Pre-commit:** Antes de realizar cualquier cambio, el Agente Ingeniero debe verificar que no se hayan introducido credenciales hardcodeadas en el código fuente. Todo manejo de configuración sensible debe hacerse a través de variables de entorno o lectura segura del path `~/.config/maximus-cli`, sin exponer el contenido real en el código.

### 🌿 Flujo de Control de Versiones (Git Workflow)
* **Aislamiento Obligatorio:** Ningún agente tiene permitido trabajar o realizar commits directamente en la rama principal (`main` o `master`). 
* **Creación de Ramas:** Al iniciar una nueva tarea o feature, los agentes deben utilizar sus herramientas de terminal para crear una nueva rama de trabajo descriptiva (por ejemplo: `git checkout -b feature/nombre-de-la-tarea` o `fix/descripcion-del-error`).
* **Commits Atómicos:** Una vez que el código ha sido escrito y el Agente Revisor ha confirmado que el `Makefile` compila correctamente, los agentes deben registrar los cambios en la rama actual mediante commits con mensajes claros, concisos y descriptivos sobre la funcionalidad implementada.