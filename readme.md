# VM Desktop Streamer

Solution de partage d'écran en temps réel via navigateur web, développée en Go avec WebSocket.

## Fonctionnalités

- Partage d'écran haute performance jusqu'à 120 FPS
- Interface web accessible depuis n'importe quel navigateur
- Support multi-écrans avec sélection individuelle
- Qualité d'image adaptative selon le FPS choisi
- Contrôle FPS dynamique en temps réel
- Vue responsive qui s'adapte automatiquement à la taille d'écran

## Installation

### Prérequis

- Go 1.25.1 ou supérieur
- Compilateur C requis pour la capture d'écran native :
  - **Windows** : Visual Studio Build Tools ou MinGW
  - **Linux** : `sudo apt install gcc`
  - **macOS** : `xcode-select --install`

### Installation

```bash
# Créer le projet
mkdir vm-desktop-streamer
cd vm-desktop-streamer

# Copier le code dans main.go
# Créer le fichier go.mod :
echo "module vm-desktop-streamer

go 1.25.1

require (
    github.com/gorilla/websocket v1.5.0
    github.com/kbinani/screenshot v0.0.0-20210720154843-7d3a670d8329
)" > go.mod

# Installer les dépendances
go mod tidy

# Lancer l'application
go run main.go
```

## Utilisation

1. Lancer l'application : `go run main.go`
2. Ouvrir un navigateur à l'adresse : `http://localhost:8080`
3. Cliquer sur "Connect" pour démarrer le streaming
4. Utiliser les contrôles pour ajuster le FPS et changer d'écran

### Contrôles disponibles

- **Boutons FPS** : 5, 10, 15, 30, 60, 120 FPS
- **Sélection d'écran** : All (tous), 1, 2, 3 (écrans individuels)
- **Double-clic** : Basculer en plein écran
- **Touches clavier** :
  - `F` ou `Escape` : Plein écran
  - `1-4` : Sélection d'écran

### Personnalisation

Changer le port d'écoute :
```bash
go run main.go 9000  # Utilise le port 9000
```

## Performance

La solution utilise une capture d'écran native optimisée qui permet :
- Temps de capture : 5-15ms (vs 500ms avec PowerShell classique)
- FPS réel proche du FPS configuré
- Diffusion binaire JPEG sans encodage base64

## Architecture technique

- **Backend** : Go avec capture native via `kbinani/screenshot`
- **Communication** : WebSocket avec transmission binaire
- **Frontend** : HTML5/JavaScript avec gestion optimisée des blobs
- **Compression** : JPEG avec qualité adaptative selon le FPS

## Sécurité

ATTENTION : Cette version est configurée pour le développement. En production :

1. Modifier la fonction `CheckOrigin` dans le code pour restreindre les origines autorisées
2. Ajouter une authentification
3. Utiliser HTTPS avec certificats SSL
4. Configurer un firewall approprié