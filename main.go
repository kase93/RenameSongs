// RenameSongs renombra archivos de audio a "Artista - Título.ext"
// usando los metadatos incrustados en cada archivo.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/dhowden/tag"
)

const (
	nombreApp = "renombrar_canciones"
	version   = "1.1.0"
)

// Extensiones de audio que intentamos procesar.
var extensionesValidas = map[string]bool{
	".mp3": true, ".flac": true, ".m4a": true, ".mp4": true,
	".ogg": true, ".oga": true, ".opus": true, ".wav": true,
	".aac": true, ".wma": true, ".aiff": true, ".ape": true,
}

// Sustituciones de caracteres no permitidos en nombres de archivo (Windows).
var sustituciones = map[string]string{
	"\\": "-",
	"/":  "-",
	":":  " -",
	"*":  "x",
	"?":  "",
	"\"": "'",
	"<":  "(",
	">":  ")",
	"|":  "-",
}

var (
	reControl  = regexp.MustCompile(`[\x00-\x1f\x7f]`)
	reEspacios = regexp.MustCompile(`\s+`)
	reGuiones  = regexp.MustCompile(`\s+-\s+`)
	// Coincide con nombres como "Nombre (2)", "Nombre (12)".
	reSufijoNumerado  = regexp.MustCompile(`^.+?\s+\(\d+\)$`)
	subSufijoNumerado = regexp.MustCompile(`\s+\(\d+\)$`)
)

// Nombres reservados por Windows (CON, PRN, AUX, NUL, COM1..9, LPT1..9).
var nombresReservados = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

func main() {
	dryRun := flag.Bool("dry-run", false, "muestra los cambios sin renombrar (alias: -n)")
	dryRunCorto := flag.Bool("n", false, "alias de --dry-run")
	recursivo := flag.Bool("recursive", false, "procesa también las subcarpetas (alias: -r)")
	recursivoCorto := flag.Bool("r", false, "alias de --recursive")
	mostrarVersion := flag.Bool("version", false, "muestra la versión y sale")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Uso: %s [carpeta] [--dry-run] [--recursive]\n", nombreApp)
		fmt.Fprintln(flag.CommandLine.Output(), "Sin carpeta usa Descargas.")
		flag.PrintDefaults()
	}

	// Permite escribir las opciones antes o después de la carpeta:
	// el paquete flag deja de leer opciones en el primer argumento posicional.
	if err := flag.CommandLine.Parse(reordenarArgs(os.Args[1:])); err != nil {
		os.Exit(2)
	}

	if *mostrarVersion {
		fmt.Printf("%s %s\n", nombreApp, version)
		return
	}

	carpeta := flag.Arg(0)
	sinCarpeta := carpeta == ""
	if sinCarpeta {
		carpeta = carpetaDescargas()
	}

	fmt.Printf("Carpeta: %s\n\n", carpeta)

	est := procesar(carpeta, *dryRun || *dryRunCorto, *recursivo || *recursivoCorto)

	fmt.Printf("\nResumen: %d renombrados, %d omitidos, %d errores\n",
		est.renombrados, est.omitidos, est.errores)

	// Pausa al ejecutar con doble clic (sin carpeta) para que la consola no se
	// cierre antes de leer el resultado; no bloquea si stdin no es interactivo.
	if sinCarpeta {
		pausa()
	}

	if est.errores > 0 {
		os.Exit(1)
	}
}

// reordenarArgs coloca las opciones delante para que flag las reconozca
// aunque se escriban después de la carpeta.
func reordenarArgs(args []string) []string {
	opciones := make([]string, 0, len(args))
	posicionales := make([]string, 0, len(args))
	for _, a := range args {
		if strings.HasPrefix(a, "-") && a != "-" {
			opciones = append(opciones, a)
			continue
		}
		posicionales = append(posicionales, a)
	}
	return append(opciones, posicionales...)
}

func carpetaDescargas() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads")
}

// limpiarNombre sustituye caracteres no válidos y normaliza espacios.
func limpiarNombre(texto string) string {
	texto = strings.TrimSpace(texto)
	for original, sustituto := range sustituciones {
		texto = strings.ReplaceAll(texto, original, sustituto)
	}
	texto = reControl.ReplaceAllString(texto, "")
	texto = reEspacios.ReplaceAllString(texto, " ")
	texto = strings.Trim(texto, " .")
	return texto
}

// normalizarNombre limpia y evita nombres reservados o vacíos.
func normalizarNombre(texto string) string {
	texto = limpiarNombre(texto)
	if texto == "" {
		return "_"
	}
	base := strings.ToUpper(texto)
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	if nombresReservados[base] {
		texto = "_" + texto
	}
	return texto
}

// limitarNombre recorta el nombre para no superar el límite de Windows (255 bytes).
func limitarNombre(nombre string) string {
	const limite = 255
	if len(nombre) <= limite {
		return nombre
	}
	ext := filepath.Ext(nombre)
	base := strings.TrimSuffix(nombre, ext)
	presupuesto := limite - len(ext)
	for len(base) > presupuesto {
		_, size := utf8.DecodeLastRuneInString(base)
		base = base[:len(base)-size]
	}
	return strings.Trim(base, " .") + ext
}

type estado struct {
	renombrados, omitidos, errores int
}

// procesar recorre la carpeta y renombra los archivos de audio encontrados.
func procesar(carpeta string, dryRun, recursivo bool) estado {
	var est estado

	info, err := os.Stat(carpeta)
	if err != nil || !info.IsDir() {
		fmt.Printf("La carpeta no existe o no se puede leer: %s\n", carpeta)
		est.errores++
		return est
	}

	var archivos []string
	agregar := func(ruta string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("⚠ No se pudo acceder a %s: %v\n", ruta, err)
			est.errores++
			return nil
		}
		if info.IsDir() {
			if !recursivo && ruta != carpeta {
				return filepath.SkipDir
			}
			return nil
		}
		if extensionesValidas[strings.ToLower(filepath.Ext(ruta))] {
			archivos = append(archivos, ruta)
		}
		return nil
	}

	if err := filepath.Walk(carpeta, agregar); err != nil {
		fmt.Printf("Error recorriendo la carpeta: %v\n", err)
		est.errores++
		return est
	}

	if len(archivos) == 0 {
		fmt.Println("No se encontraron archivos de audio en la carpeta.")
		return est
	}

	// Reserva de nombres destino ya usados en esta ejecución (comparación sin distinguir mayúsculas).
	usados := map[string]bool{}

	for _, ruta := range archivos {
		artista, titulo, err := extraer(ruta)
		if err != nil {
			fmt.Printf("⚠ No se pudo leer %s: %v\n", filepath.Base(ruta), err)
			est.errores++
			continue
		}
		if artista == "" || titulo == "" {
			fmt.Printf("✗ Sin metadatos suficientes, se omite: %s\n", filepath.Base(ruta))
			est.omitidos++
			continue
		}

		ext := strings.ToLower(filepath.Ext(ruta))
		nombre := limitarNombre(fmt.Sprintf("%s - %s%s",
			normalizarNombre(artista), normalizarNombre(titulo), ext))
		destino := filepath.Join(filepath.Dir(ruta), nombre)

		if yaNombrado(ruta, destino) {
			est.omitidos++
			continue
		}

		destino = nombreLibre(destino, usados)

		if dryRun {
			fmt.Printf("[Simulación] %s  ->  %s\n", filepath.Base(ruta), filepath.Base(destino))
			est.renombrados++
			continue
		}

		if err := os.Rename(ruta, destino); err != nil {
			fmt.Printf("⚠ Error al renombrar %s: %v\n", filepath.Base(ruta), err)
			est.errores++
			continue
		}
		fmt.Printf("✓ Renombrado: %s  ->  %s\n", filepath.Base(ruta), filepath.Base(destino))
		est.renombrados++
	}

	return est
}

// extraer devuelve artista y título desde los metadatos; si faltan, intenta
// deducirlos del nombre actual del archivo ("Artista - Título.ext").
func extraer(ruta string) (string, string, error) {
	var artista, titulo string

	f, err := os.Open(ruta)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	if meta, err := tag.ReadFrom(f); err == nil {
		artista = strings.TrimSpace(meta.Artist())
		titulo = strings.TrimSpace(meta.Title())
	} else if err != nil && err != tag.ErrNoTagsFound {
		// No es un error fatal: probamos con el nombre del archivo.
		fmt.Printf("⚠ Sin metadatos legibles en %s: %v\n", filepath.Base(ruta), err)
	}

	if artista == "" || titulo == "" {
		if a, t := desdeNombreArchivo(ruta); a != "" && t != "" {
			if artista == "" {
				artista = a
			}
			if titulo == "" {
				titulo = t
			}
		}
	}

	return artista, titulo, nil
}

// desdeNombreArchivo intenta separar "Artista - Título" del nombre del archivo.
func desdeNombreArchivo(ruta string) (string, string) {
	base := strings.TrimSuffix(filepath.Base(ruta), filepath.Ext(ruta))
	base = strings.TrimSpace(base)

	if partes := strings.SplitN(base, " - ", 2); len(partes) == 2 {
		return strings.TrimSpace(partes[0]), strings.TrimSpace(partes[1])
	}
	if partes := reGuiones.Split(base, 2); len(partes) == 2 {
		return strings.TrimSpace(partes[0]), strings.TrimSpace(partes[1])
	}
	return "", ""
}

// nombreLibre evita sobrescribir archivos y colisiones dentro de la misma ejecución.
func nombreLibre(destino string, usados map[string]bool) string {
	clave := strings.ToLower(destino)
	if !usados[clave] && !existe(destino) {
		usados[clave] = true
		return destino
	}

	ext := filepath.Ext(destino)
	base := strings.TrimSuffix(destino, ext)
	for i := 2; ; i++ {
		candidato := fmt.Sprintf("%s (%d)%s", base, i, ext)
		claveCandidato := strings.ToLower(candidato)
		if usados[claveCandidato] || existe(candidato) {
			continue
		}
		usados[claveCandidato] = true
		fmt.Printf("⚠ Ya existe un archivo con el nombre destino, se usa: %s\n", filepath.Base(candidato))
		return candidato
	}
}

func existe(ruta string) bool {
	_, err := os.Stat(ruta)
	return err == nil
}

// yaNombrado indica si el archivo ya se llama como el destino o como una
// variante numerada del mismo ("Nombre (2).ext"), para no renombrarlo en bucle.
func yaNombrado(ruta, destino string) bool {
	if strings.EqualFold(filepath.Clean(ruta), filepath.Clean(destino)) {
		return true
	}
	if !strings.EqualFold(filepath.Ext(ruta), filepath.Ext(destino)) {
		return false
	}
	baseDestino := strings.TrimSuffix(filepath.Base(destino), filepath.Ext(destino))
	baseActual := strings.TrimSpace(strings.TrimSuffix(filepath.Base(ruta), filepath.Ext(ruta)))
	if strings.EqualFold(baseActual, baseDestino) {
		return true
	}
	return reSufijoNumerado.MatchString(baseActual) &&
		strings.EqualFold(subSufijoNumerado.ReplaceAllString(baseActual, ""), baseDestino)
}

func pausa() {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return
	}
	fmt.Print("\nPulsa Enter para salir...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
