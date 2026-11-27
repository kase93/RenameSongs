package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLimpiarNombre(t *testing.T) {
	casos := map[string]string{
		"AC/DC":            "AC-DC",
		"Back: In Black":   "Back - In Black",
		`Rock "n" Roll`:    "Rock 'n' Roll",
		"  muchos   espacios  ": "muchos espacios",
		"punto final.":     "punto final",
		"a*b?c":            "axbc",
	}
	for entrada, esperado := range casos {
		if obtenido := limpiarNombre(entrada); obtenido != esperado {
			t.Errorf("limpiarNombre(%q) = %q, se esperaba %q", entrada, obtenido, esperado)
		}
	}
}

func TestNormalizarNombreReservados(t *testing.T) {
	if got := normalizarNombre("CON"); got != "_CON" {
		t.Errorf("normalizarNombre(\"CON\") = %q, se esperaba \"_CON\"", got)
	}
	if got := normalizarNombre("nul.mp3"); got != "_nul.mp3" {
		t.Errorf("normalizarNombre(\"nul.mp3\") = %q, se esperaba \"_nul.mp3\"", got)
	}
	if got := normalizarNombre("   "); got != "_" {
		t.Errorf("normalizarNombre vacío = %q, se esperaba \"_\"", got)
	}
}

func TestDesdeNombreArchivo(t *testing.T) {
	if a, ti := desdeNombreArchivo("Daft Punk - Get Lucky.mp3"); a != "Daft Punk" || ti != "Get Lucky" {
		t.Errorf("desdeNombreArchivo = (%q, %q)", a, ti)
	}
	if a, ti := desdeNombreArchivo("Daft Punk - Get Lucky - Remix.flac"); a != "Daft Punk" || ti != "Get Lucky - Remix" {
		t.Errorf("desdeNombreArchivo = (%q, %q)", a, ti)
	}
	if a, ti := desdeNombreArchivo("sintitulo.mp3"); a != "" || ti != "" {
		t.Errorf("desdeNombreArchivo sin separador = (%q, %q)", a, ti)
	}
}

func TestYaNombrado(t *testing.T) {
	dir := t.TempDir()
	casos := []struct {
		actual, destino string
		esperado        bool
	}{
		{"Daft Punk - Get Lucky.mp3", "Daft Punk - Get Lucky.mp3", true},
		{"daft punk - get lucky.MP3", "Daft Punk - Get Lucky.mp3", true},
		{"Daft Punk - Get Lucky (2).mp3", "Daft Punk - Get Lucky.mp3", true},
		{"Daft Punk - Get Lucky (12).mp3", "Daft Punk - Get Lucky.mp3", true},
		{"track01.mp3", "Daft Punk - Get Lucky.mp3", false},
		{"Daft Punk - Get Lucky (2).flac", "Daft Punk - Get Lucky.mp3", false},
	}
	for _, c := range casos {
		actual := filepath.Join(dir, c.actual)
		destino := filepath.Join(dir, c.destino)
		if got := yaNombrado(actual, destino); got != c.esperado {
			t.Errorf("yaNombrado(%q, %q) = %v, se esperaba %v", c.actual, c.destino, got, c.esperado)
		}
	}
}

func TestNombreLibreNoSobrescribe(t *testing.T) {
	dir := t.TempDir()
	destino := filepath.Join(dir, "Artista - Tema.mp3")
	if err := os.WriteFile(destino, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	usados := map[string]bool{}
	libre := nombreLibre(destino, usados)
	if filepath.Base(libre) != "Artista - Tema (2).mp3" {
		t.Errorf("nombreLibre = %q, se esperaba \"Artista - Tema (2).mp3\"", filepath.Base(libre))
	}

	// Un segundo archivo con el mismo nombre debe recibir (3).
	if segundo := nombreLibre(destino, usados); filepath.Base(segundo) != "Artista - Tema (3).mp3" {
		t.Errorf("segundo nombreLibre = %q, se esperaba \"Artista - Tema (3).mp3\"", filepath.Base(segundo))
	}
}

func TestReordenarArgs(t *testing.T) {
	got := reordenarArgs([]string{"carpeta", "--dry-run", "-r"})
	esperado := []string{"--dry-run", "-r", "carpeta"}
	if len(got) != len(esperado) {
		t.Fatalf("reordenarArgs = %v, se esperaba %v", got, esperado)
	}
	for i := range got {
		if got[i] != esperado[i] {
			t.Fatalf("reordenarArgs = %v, se esperaba %v", got, esperado)
		}
	}
}
