package launch

import (
	"strings"
)

// junkKeywords contient des mots-clés qui indiquent généralement un processus secondaire
// ou un utilitaire de rapport d'erreur qui ne doit pas être comptabilisé comme le jeu lui-même.
var junkKeywords = []string{
	"crashpad_handler",
	"crashreportclient",
	"crashreporter",
	"errorreporting",
	"unitycrashhandler",
	"unrealsync",
	"unrealcefsubprocess",
	"feedback",
	"bugreport",
	"crashreport",
	"werfault",
	"dumpit",
	"easyanticheat",
	"battleye",
	"epicwebhelper",
	"steamwebhelper",
	"socialclub",
	"overlay",
}

// IsJunkProcess vérifie si le nom d'un processus ou son chemin correspond à des utilitaires connus
// qui ne sont pas des jeux (crash handlers, reporters, etc.).
func IsJunkProcess(names ...string) bool {
	for _, name := range names {
		if name == "" {
			continue
		}
		lowerName := strings.ToLower(name)

		for _, keyword := range junkKeywords {
			if strings.Contains(lowerName, keyword) {
				return true
			}
		}
	}

	return false
}
