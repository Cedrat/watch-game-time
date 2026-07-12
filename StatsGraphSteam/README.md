# StatsGraphSteam

Mini outil en Go pour visualiser le **taux d'abandon** des succès (achievements)
d'un jeu Steam, via une interface web moderne avec drag & drop.

## Fonctionnalités

- Récupération des succès d'un jeu Steam via l'API officielle
  (`GetSchemaForGame` + `GetGlobalAchievementPercentagesForApp`).
- Paramétrage de la clé API Steam directement depuis l'interface web
  (stockée localement dans `config.json` à côté de l'exécutable).
- Sélection des succès par **clic** ou **glisser-déposer** vers la zone de graphique.
- Graphique horizontal (Chart.js) avec axe au choix :
  - Taux d'abandon (`100 - % global`)
  - Taux de complétion (`% global`)
- Tri automatique, filtrage par texte, tooltips détaillés.
- Thème sombre moderne.

## Prérequis

- Go 1.22+
- Une clé API Steam : https://steamcommunity.com/dev/apikey

## Lancement

```bash
go run .
```

Puis ouvrez http://localhost:8420 dans votre navigateur.

## Utilisation

1. Onglet **Paramètres** → collez votre clé API Steam → Enregistrer.
2. Onglet **Analyse** → saisissez un AppID (ex: `730` pour CS2, `440` pour TF2).
3. Cliquez sur **Charger**.
4. Cliquez sur les succès ou glissez-les vers la zone de graphique.
5. Choisissez l'axe Y (abandon / complétion).

## Structure

```
.
├── go.mod
├── main.go        # serveur HTTP + routes API
├── steam.go       # client API Steam
├── config.go      # persistance de la clé API
└── static/
    ├── index.html
    ├── styles.css
    └── app.js     # UI, drag & drop, Chart.js
```

## Notes

- L'AppID doit être numérique (ex: `730`, pas `CS2`).
- Les pourcentages globaux sont des moyennes mondiales Steam : ils reflètent
  la part des joueurs ayant débloqué chaque succès au moins une fois.
- Le "taux d'abandon" est défini ici comme `100 - % global` : c'est donc la part
  de joueurs n'ayant **pas** débloqué le succès.
