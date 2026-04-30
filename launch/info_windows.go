package launch

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	versionDll             = syscall.NewLazyDLL("version.dll")
	getFileVersionInfoSize = versionDll.NewProc("GetFileVersionInfoSizeW")
	getFileVersionInfo     = versionDll.NewProc("GetFileVersionInfoW")
	verQueryValue          = versionDll.NewProc("VerQueryValueW")
)

// GetFriendlyName tente de trouver un nom lisible pour un exécutable en utilisant les métadonnées Windows.
// Si aucune métadonnée n'est trouvée, il renvoie le nom du fichier nettoyé.
func GetFriendlyName(path string) string {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return cleanFilename(path)
	}

	// 1. Récupérer la taille des informations de version
	var handle uintptr
	size, _, _ := getFileVersionInfoSize.Call(uintptr(unsafe.Pointer(pathPtr)), uintptr(unsafe.Pointer(&handle)))
	if size == 0 {
		return cleanFilename(path)
	}

	// 2. Récupérer les données de version
	data := make([]byte, size)
	ret, _, _ := getFileVersionInfo.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		size,
		uintptr(unsafe.Pointer(&data[0])),
	)
	if ret == 0 {
		return cleanFilename(path)
	}

	// 3. Déterminer la langue et le charset (Translation) pour interroger les chaînes
	var valuePtr uintptr
	var valueLen uint32
	subBlock, _ := syscall.UTF16PtrFromString("\\VarFileInfo\\Translation")
	ret, _, _ = verQueryValue.Call(
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(subBlock)),
		uintptr(unsafe.Pointer(&valuePtr)),
		uintptr(unsafe.Pointer(&valueLen)),
	)

	// Par défaut on essaie l'anglais Unicode si on ne trouve pas la translation
	langCharset := "040904b0"
	if ret != 0 && valueLen >= 4 {
		lang := *(*uint16)(unsafe.Pointer(valuePtr))
		charset := *(*uint16)(unsafe.Pointer(valuePtr + 2))
		langCharset = fmt.Sprintf("%04x%04x", lang, charset)
	}

	// 4. Tenter d'extraire le ProductName ou la FileDescription
	fields := []string{"ProductName", "FileDescription"}
	for _, field := range fields {
		query := fmt.Sprintf("\\StringFileInfo\\%s\\%s", langCharset, field)
		queryPtr, _ := syscall.UTF16PtrFromString(query)

		ret, _, _ = verQueryValue.Call(
			uintptr(unsafe.Pointer(&data[0])),
			uintptr(unsafe.Pointer(queryPtr)),
			uintptr(unsafe.Pointer(&valuePtr)),
			uintptr(unsafe.Pointer(&valueLen)),
		)

		if ret != 0 && valueLen > 0 {
			// Convertir le pointeur UTF16 vers une string Go
			name := syscall.UTF16ToString((*[1 << 16]uint16)(unsafe.Pointer(valuePtr))[:valueLen])
			name = strings.TrimSpace(name)
			if name != "" {
				return name
			}
		}
	}

	return cleanFilename(path)
}

// cleanFilename retire l'extension et nettoie les caractères spéciaux du nom de fichier
func cleanFilename(path string) string {
	name := filepath.Base(path)
	// Enlever l'extension (.exe)
	name = strings.TrimSuffix(name, filepath.Ext(name))

	// Remplacer les séparateurs courants par des espaces
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")

	// Mettre la première lettre en majuscule
	if len(name) > 0 {
		return strings.ToUpper(name[:1]) + name[1:]
	}
	return name
}
