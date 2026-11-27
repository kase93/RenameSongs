# RenameSongs

Renombra archivos de audio a `Artista - Título.ext` usando los metadatos
incrustados en cada archivo. Funciona en Windows (probado en Windows 10/11),
macOS y Linux.

## Uso

```
renombrar_canciones.exe                       # renombra la carpeta Descargas
renombrar_canciones.exe "C:\Musica\EDM"       # renombra esa carpeta
renombrar_canciones.exe "C:\Musica\EDM" -n    # simulación, no toca archivos
renombrar_canciones.exe "C:\Musica" -r        # incluye subcarpetas
```

Las opciones se pueden escribir antes o después de la ruta.

| Opción | Alias | Descripción |
| --- | --- | --- |
| `--dry-run` | `-n` | Muestra los cambios sin renombrar nada |
| `--recursive` | `-r` | Procesa también las subcarpetas |
| `--version` | | Muestra la versión y sale |

## Qué hace

1. Lee las etiquetas del archivo con [dhowden/tag](https://github.com/dhowden/tag).
2. Si faltan artista o título, intenta deducirlos del nombre actual (`Artista - Título.ext`).
3. Limpia caracteres inválidos de Windows (`/`, `:`, `*`, `?`, `"`, `<`, `>`, `|`).
4. Renombra el archivo y muestra un resumen al final.

Extensiones soportadas: `.mp3`, `.flac`, `.m4a`, `.mp4`, `.ogg`, `.oga`, `.opus`,
`.wav`, `.aac`, `.wma`, `.aiff`, `.ape`.

## Seguridad

- Nunca sobrescribe: si el destino ya existe añade un número (`(2)`, `(3)`, ...).
- Es idempotente: un archivo ya bien nombrado se omite en ejecuciones posteriores.
- Evita nombres reservados de Windows (`CON`, `NUL`, `COM1`, ...).
- Recorta nombres largos para no superar el límite de 255 bytes.

## Compilar

```
go build -trimpath -ldflags "-s -w" -o renombrar_canciones.exe .
```

## Descargar

El ejecutable para Windows está en la sección
[Releases](../../releases/latest).

## Licencia

MIT
