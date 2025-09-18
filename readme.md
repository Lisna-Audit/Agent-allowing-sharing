# VM Desktop Streamer

Solution de partage d'écran et de contrôle en temps réel via navigateur web, développée en Go avec WebSocket.

## Fonctionnalités

- Partage d'écran haute performance jusqu'à 120 FPS
- Interface web accessible depuis n'importe quel navigateur
- Support multi-écrans avec sélection individuelle
- Qualité d'image adaptative selon le FPS choisi
- Contrôle FPS dynamique en temps réel
- Vue responsive qui s'adapte automatiquement à la taille d'écran
- Contrôle souris et clavier à distance
- Synchronisation presse-papiers (VM ↔ navigateur)
- Défilement molette (scroll)
- Gestion connect / disconnect côté client
- Plein écran interactif via double-clic ou touche F/Escape

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
4. Cliquer sur "Enable Control" pour activer le contrôle souris/clavier
5. Utiliser les contrôles pour ajuster le FPS et changer d'écran
6. "Sync Clipboard" permet de synchroniser le presse-papiers manuellement

### Contrôles disponibles

- **Boutons FPS** : 5, 10, 15, 30, 60, 120 FPS
- **Sélection d'écran** : All (tous), 1, 2, 3 (écrans individuels)
- **Double-clic** : Basculer en plein écran
- **Molette** : Scroll vertical dans la VM
- **Touches clavier** :
  - `F` ou `Escape` : Plein écran
  - `1-4` avec Ctrl : Sélection d'écran
  - `Ctrl+C / Ctrl+V` : Copier-coller avec synchro automatique du presse-papiers
- **Souris** :
  - Clic gauche/droit/milieu
  - Glisser-déposer (drag)
  - Déplacement fluide (throttling 60Hz)

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
- Qualité JPEG adaptative (réduite automatiquement quand FPS élevé pour garder la fluidité)
- Affichage via `<canvas>` et `createImageBitmap` pour réduire la latence

## Architecture technique

- **Backend** : Go avec capture native via `kbinani/screenshot`
- **Communication** : WebSocket avec transmission binaire + events JSON
- **Frontend** : HTML5/JavaScript avec canvas, gestion des événements souris/clavier
- **Compression** : JPEG avec qualité adaptative selon le FPS
- **Clipboard** : gestion VM ↔ navigateur via WebSocket
- **Sécurité connexion** : gestion manuelle connect/disconnect côté client

## Sécurité

ATTENTION : Cette version est configurée pour le développement. En production :

1. Modifier la fonction `CheckOrigin` dans le code pour restreindre les origines autorisées
2. Ajouter une authentification
3. Utiliser HTTPS avec certificats SSL
4. Configurer un firewall approprié


------------------------------

## VM Linux configuration

### 1. Mise à jour système
```bash
sudo apt update && sudo apt upgrade -y
```

### 2. Environnement graphique
```bash
# Installer GNOME (si pas déjà fait)
sudo apt install gnome-core gdm3 -y

# Redémarrer pour activer GDM
sudo systemctl restart gdm3
```

### 3. Au login : Choisir "GNOME on Xorg"

1. Cliquer sur l'icône engrenage à côté du bouton de connexion
2. Sélectionner "GNOME on Xorg" au lieu de "GNOME"
3. Se connecter normalement

### 4. Dépendances Go et compilation

```bash
# Go
sudo apt install golang-go -y

# Compilateur C (requis par kbinani/screenshot)
sudo apt install gcc libc6-dev -y

# Dépendances X11
sudo apt install libx11-dev libxrandr-dev libxinerama-dev libxcursor-dev libxfixes-dev -y
```

### 5. Outils de contrôle
```bash
# xdotool pour contrôle souris/clavier
sudo apt install xdotool xclip -y

# Outils de capture d'écran (alternatives)
sudo apt install scrot imagemagick maim -y
```

### 6. Configuration réseau (si VM)
```bash
# Autoriser le port 8080
sudo ufw allow 8080

# Ou désactiver le firewall temporairement
sudo ufw disable
```

### 7. Test de fonctionnement
```bash
# Vérifier l'environnement X11
echo $DISPLAY          # Doit afficher :0
echo $XDG_SESSION_TYPE  # Doit afficher x11

# Tester les outils
xdotool getmouselocation
scrot test.png
go version
```

### 8. Lancement de l'application

```bash
# Dans le dossier du projet
go mod tidy
go run main.go

# Accès depuis une autre machine
http://IP_DE_LA_VM:8080
```

## Windows configuration

### 1. Installation de Go
Télécharger et installer Go depuis le site officiel :  
[https://go.dev/dl/](https://go.dev/dl/)

### 2. Compilateur C
Installer un compilateur C, requis par `kbinani/screenshot` :
- Option 1 : [Visual Studio Build Tools](https://visualstudio.microsoft.com/visual-cpp-build-tools/)  
- Option 2 : [MinGW-w64](http://mingw-w64.org/doku.php)

### 3. Dépendances natives
- **PowerShell** : déjà inclus dans Windows, utilisé pour le contrôle souris/clavier et le presse-papiers.  
- **NirCmd** (optionnel) : peut être installé pour compléter certaines actions souris si PowerShell échoue.  
  Téléchargement : [https://www.nirsoft.net/utils/nircmd.html](https://www.nirsoft.net/utils/nircmd.html)

---

## macOS configuration

### 1. Installation de Go
```bash
brew install go
```
Ou télécharger depuis le site officiel : [https://go.dev/dl/](https://go.dev/dl/)

### 2. Compilateur C
Installer les outils de développement Apple :
```bash
xcode-select --install
```

### 3. Dépendances natives
- **osascript** : inclus dans macOS, utilisé pour le contrôle souris/clavier.  
- **pbcopy / pbpaste** : inclus dans macOS, utilisés pour la gestion du presse-papiers.  

Aucune installation supplémentaire n’est nécessaire.
