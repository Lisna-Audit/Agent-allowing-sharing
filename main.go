package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kbinani/screenshot"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // return true = ACCEPTE TOUTES LES CONNEXIONS depuis n'importe quel site web !
	},
	// =============> TODO 1
	// 	CheckOrigin: func(r *http.Request) bool {
	//     origin := r.Header.Get("Origin")
	//     allowedOrigins := []string{
	//         "http://localhost:8080",
	//         "https://votre-domaine.com",
	//         "https://vm-sandbox.entreprise.com",
	//     }

	//     for _, allowed := range allowedOrigins {
	//         if origin == allowed {
	//             return true
	//         }
	//     }
	//     log.Printf("🚨 Origine refusée: %s", origin)
	//     return false
	// },
}

type ScreenStreamer struct {
	clients       map[*websocket.Conn]bool
	currentScreen int
	currentFPS    int
	fpsChanged    chan int
}

func NewScreenStreamer() *ScreenStreamer {
	return &ScreenStreamer{
		clients:       make(map[*websocket.Conn]bool),
		currentScreen: -1, // -1 = tous les écrans
		currentFPS:    10, // FPS par défaut
		fpsChanged:    make(chan int, 1),
	}
}

func (s *ScreenStreamer) addClient(conn *websocket.Conn) {
	s.clients[conn] = true
	log.Printf("Client connecté. Total: %d", len(s.clients))
}

func (s *ScreenStreamer) removeClient(conn *websocket.Conn) {
	delete(s.clients, conn)
	conn.Close()
	log.Printf("Client déconnecté. Total: %d", len(s.clients))
}

// Capture d'écran native ultra-rapide avec kbinani/screenshot
func captureScreen(screenIndex int) (image.Image, error) {
	var img *image.RGBA
	var err error

	if screenIndex == -1 {
		// Capture tous les écrans (écran virtuel)
		if screenshot.NumActiveDisplays() == 0 {
			return nil, fmt.Errorf("aucun écran détecté")
		}

		// Calculer les bounds de tous les écrans
		bounds := screenshot.GetDisplayBounds(0)
		for i := 1; i < screenshot.NumActiveDisplays(); i++ {
			bounds = bounds.Union(screenshot.GetDisplayBounds(i))
		}
		img, err = screenshot.CaptureRect(bounds)
	} else {
		// Capture d'un écran spécifique
		if screenIndex >= screenshot.NumActiveDisplays() {
			return nil, fmt.Errorf("écran %d non trouvé (max: %d)", screenIndex, screenshot.NumActiveDisplays()-1)
		}
		bounds := screenshot.GetDisplayBounds(screenIndex)
		img, err = screenshot.CaptureRect(bounds)
	}

	if err != nil {
		return nil, fmt.Errorf("erreur capture: %v", err)
	}
	return img, nil
}

// Diffusion optimisée en binaire
func (s *ScreenStreamer) broadcastImage(img image.Image, quality int) error {
	var buf bytes.Buffer

	// Encodage JPEG optimisé
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	if err != nil {
		return fmt.Errorf("erreur encodage: %v", err)
	}

	// Diffusion binaire à tous les clients
	data := buf.Bytes()
	for client := range s.clients {
		err := client.WriteMessage(websocket.BinaryMessage, data)
		if err != nil {
			log.Printf("Erreur envoi client: %v", err)
			s.removeClient(client)
		}
	}
	return nil
}

// Handler WebSocket pour streaming
func (s *ScreenStreamer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// =============> TODO 2
	// // Vérifier token d'auth
	// token := r.URL.Query().Get("token")
	// if token != "votre-secret-token" {
	//     http.Error(w, "Unauthorized", 401)
	//     return
	// }
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Erreur upgrade WebSocket: %v", err)
		return
	}

	s.addClient(conn)

	// Écouter les messages du client
	go func() {
		defer s.removeClient(conn)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				break
			}

			// Traiter les commandes du client
			command := strings.TrimSpace(string(message))
			switch {
			case command == "refresh":
				// Force un refresh
				continue
			case strings.HasPrefix(command, "screen:"):
				// Changer d'écran: "screen:0", "screen:1", "screen:all"
				screenStr := strings.TrimPrefix(command, "screen:")
				if screenStr == "all" {
					s.currentScreen = -1
					log.Printf("Écran changé vers: Tous les écrans")
				} else {
					var screenIndex int
					if n, _ := fmt.Sscanf(screenStr, "%d", &screenIndex); n == 1 {
						if screenIndex < screenshot.NumActiveDisplays() {
							s.currentScreen = screenIndex
							log.Printf("Écran changé vers: %d", screenIndex)
						}
					}
				}
			case strings.HasPrefix(command, "fps:"):
				// Changer FPS: "fps:15", "fps:30", etc.
				fpsStr := strings.TrimPrefix(command, "fps:")
				var fps int
				if n, _ := fmt.Sscanf(fpsStr, "%d", &fps); n == 1 && fps > 0 && fps <= 120 {
					s.currentFPS = fps
					// Notifier le changement de FPS
					select {
					case s.fpsChanged <- fps:
					default:
					}
					log.Printf("FPS changé vers: %d", fps)
				}
			}
		}
	}()
}

// Boucle de capture et diffusion avec FPS dynamique
func (s *ScreenStreamer) startStreaming() {
	currentFPS := s.currentFPS
	ticker := time.NewTicker(time.Second / time.Duration(currentFPS))
	defer ticker.Stop()

	log.Printf("Streaming démarré à %d FPS", currentFPS)
	log.Printf("Écrans détectés: %d", screenshot.NumActiveDisplays())

	frameCount := 0
	lastStatsTime := time.Now()

	for {
		select {
		case <-ticker.C:
			if len(s.clients) == 0 {
				continue // Pas de clients, pas de capture
			}

			frameStart := time.Now()

			img, err := captureScreen(s.currentScreen)
			if err != nil {
				log.Printf("Erreur capture: %v", err)
				continue
			}
			captureTime := time.Since(frameStart)

			// Qualité adaptative selon le FPS
			quality := 70
			if currentFPS >= 60 {
				quality = 45 // Qualité réduite pour très hauts FPS
			} else if currentFPS >= 30 {
				quality = 55 // Qualité réduite pour hauts FPS
			} else if currentFPS <= 5 {
				quality = 90 // Qualité élevée pour bas FPS
			}

			encodeStart := time.Now()
			err = s.broadcastImage(img, quality)
			if err != nil {
				log.Printf("Erreur diffusion: %v", err)
				continue
			}
			encodeTime := time.Since(encodeStart)

			frameTotal := time.Since(frameStart)
			frameCount++

			// Stats toutes les 5 secondes
			if time.Since(lastStatsTime) > 5*time.Second {
				actualFPS := float64(frameCount) / time.Since(lastStatsTime).Seconds()
				log.Printf("📊 FPS cible: %d | FPS réel: %.1f | Capture: %dms | Encode+Send: %dms | Total: %dms",
					currentFPS, actualFPS, captureTime.Milliseconds(), encodeTime.Milliseconds(), frameTotal.Milliseconds())
				frameCount = 0
				lastStatsTime = time.Now()
			}

		case newFPS := <-s.fpsChanged:
			// Changer le FPS en temps réel
			if newFPS != currentFPS {
				oldFPS := currentFPS
				currentFPS = newFPS
				ticker.Stop()
				ticker = time.NewTicker(time.Second / time.Duration(currentFPS))
				log.Printf("🔄 FPS changé: %d → %d (intervalle: %dms → %dms)",
					oldFPS, currentFPS, 1000/oldFPS, 1000/currentFPS)

				// Reset stats
				frameCount = 0
				lastStatsTime = time.Now()
			}
		}
	}
}

// Page web client optimisée
func serveHTML(w http.ResponseWriter, r *http.Request) {
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>VM Desktop Viewer</title>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta charset="UTF-8">
    <style>
        * { box-sizing: border-box; }
        body { 
            margin: 0; 
            padding: 10px; 
            background: #1a1a1a; 
            color: white; 
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            height: 100vh;
            overflow-x: hidden;
        }
        
        #container { 
            text-align: center; 
            height: 100%;
            display: flex;
            flex-direction: column;
        }
        
        h1 { 
            margin: 10px 0; 
            font-size: 1.5em;
        }
        
        #controls { 
            margin: 10px 0; 
            display: flex;
            flex-wrap: wrap;
            justify-content: center;
            gap: 10px;
        }
        
        button { 
            background: #333; 
            color: white; 
            border: none; 
            padding: 8px 16px; 
            cursor: pointer; 
            border-radius: 6px;
            font-size: 14px;
            transition: background 0.2s;
        }
        button:hover { background: #555; }
        button.active { background: #4CAF50; }
        button:disabled { background: #666; opacity: 0.5; cursor: not-allowed; }
        
        #status { 
            margin: 10px 0; 
            padding: 5px 10px;
            border-radius: 4px;
            display: inline-block;
            font-weight: bold;
        }
        .connected { background: #4CAF50; color: white; }
        .disconnected { background: #f44336; color: white; }
        
        #screen-container {
            flex: 1;
            display: flex;
            justify-content: center;
            align-items: center;
            padding: 10px;
            min-height: 0;
        }
        
        #screen { 
            max-width: 100%;
            max-height: 100%;
            width: auto;
            height: auto;
            border: 2px solid #333;
            border-radius: 8px;
            cursor: pointer;
            transition: transform 0.1s;
            object-fit: contain;
        }
        
        #screen:hover { 
            transform: scale(1.01); 
            border-color: #4CAF50;
        }
        
        #screen.fullscreen {
            position: fixed;
            top: 0;
            left: 0;
            width: 100vw !important;
            height: 100vh !important;
            max-width: 100vw;
            max-height: 100vh;
            z-index: 1000;
            border: none;
            border-radius: 0;
            background: black;
        }
        
        #info {
            font-size: 12px;
            color: #aaa;
            margin: 5px 0;
        }
        
        .screen-selector, .fps-selector {
            display: flex;
            gap: 5px;
            align-items: center;
        }
        
        /* Responsive */
        @media (max-width: 768px) {
            body { padding: 5px; }
            h1 { font-size: 1.2em; margin: 5px 0; }
            button { padding: 6px 12px; font-size: 12px; }
            #controls { gap: 5px; }
        }
    </style>
</head>
<body>
    <div id="container">
        <h1>VM Desktop Viewer - ULTRA FAST</h1>
        
        <div id="status" class="disconnected">Disconnected</div>
        
        <div id="controls">
            <button id="connectBtn" onclick="connect()">Connect</button>
            <button id="disconnectBtn" onclick="disconnect()">Disconnect</button>
            <button onclick="toggleFullscreen()">Fullscreen</button>
            
            <div class="screen-selector">
                <label>Screen:</label>
                <button onclick="changeScreen('all')" class="screen-btn active" data-screen="all">All</button>
                <button onclick="changeScreen(0)" class="screen-btn" data-screen="0">1</button>
                <button onclick="changeScreen(1)" class="screen-btn" data-screen="1">2</button>
                <button onclick="changeScreen(2)" class="screen-btn" data-screen="2">3</button>
            </div>
            
            <div class="fps-selector">
                <label>FPS:</label>
                <button onclick="setFPS(5)" class="fps-btn" data-fps="5">5</button>
                <button onclick="setFPS(10)" class="fps-btn active" data-fps="10">10</button>
                <button onclick="setFPS(15)" class="fps-btn" data-fps="15">15</button>
                <button onclick="setFPS(30)" class="fps-btn" data-fps="30">30</button>
                <button onclick="setFPS(60)" class="fps-btn" data-fps="60">60</button>
                <button onclick="setFPS(120)" class="fps-btn" data-fps="120">120</button>
            </div>
        </div>
        
        <div id="info">
            <span id="resolution">Resolution: --</span> | 
            <span id="fps-info">FPS: 10</span> | 
            <span id="current-screen">Current: All Screens</span>
        </div>
        
        <div id="screen-container">
            <img id="screen" alt="VM Desktop" />
        </div>
    </div>

    <script>
        let ws = null;
        let currentScreen = 'all';
        let currentFPS = 10;
        let isFullscreen = false;
        
        // Stats FPS côté client
        let frameCount = 0;
        let lastFrameTime = 0;
        let fpsDisplay = 0;
        
        const screen = document.getElementById('screen');
        const status = document.getElementById('status');
        const connectBtn = document.getElementById('connectBtn');
        const disconnectBtn = document.getElementById('disconnectBtn');

        function updateStatus(connected) {
            if (connected) {
                status.textContent = 'Connected (ULTRA FAST)';
                status.className = 'connected';
                connectBtn.disabled = true;
                disconnectBtn.disabled = false;
            } else {
                status.textContent = 'Disconnected';
                status.className = 'disconnected';
                connectBtn.disabled = false;
                disconnectBtn.disabled = true;
            }
        }

        function connect() {
            if (ws && ws.readyState === WebSocket.OPEN) return;
            
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            ws = new WebSocket(protocol + '//' + window.location.host + '/ws');
            ws.binaryType = 'arraybuffer'; // OPTIMISATION: Mode binaire
            
            ws.onopen = function() {
                updateStatus(true);
                console.log('✅ Connexion WebSocket établie');
                
                // Envoyer la configuration initiale
                if (ws.readyState === WebSocket.OPEN) {
                    ws.send('screen:' + currentScreen);
                    ws.send('fps:' + currentFPS);
                }
                
                // Reset stats
                frameCount = 0;
                lastFrameTime = Date.now();
            };
            
            ws.onmessage = function(event) {
                // OPTIMISATION: Réception binaire JPEG
                const blob = new Blob([event.data], {type: 'image/jpeg'});
                const imageUrl = URL.createObjectURL(blob);
                
                // Libérer l'ancienne URL pour éviter les fuites mémoire
                if (screen.src.startsWith('blob:')) {
                    URL.revokeObjectURL(screen.src);
                }
                
                screen.src = imageUrl;
                
                // Mesurer FPS côté client
                const now = Date.now();
                frameCount++;
                
                if (now - lastFrameTime >= 2000) { // Stats toutes les 2 secondes
                    fpsDisplay = Math.round(frameCount / ((now - lastFrameTime) / 1000));
                    document.getElementById('fps-info').textContent = 
                        'FPS: ' + currentFPS + ' (réel: ' + fpsDisplay + ')';
                    frameCount = 0;
                    lastFrameTime = now;
                }
                
                updateImageInfo();
            };
            
            ws.onclose = function() {
                updateStatus(false);
                console.log('Connexion WebSocket fermée');
                // Reconnexion automatique
                setTimeout(connect, 2000);
            };
            
            ws.onerror = function(error) {
                console.error('Erreur WebSocket:', error);
                updateStatus(false);
            };
        }

        function disconnect() {
            if (ws) {
                ws.close();
                ws = null;
            }
            if (screen.src.startsWith('blob:')) {
                URL.revokeObjectURL(screen.src);
            }
            screen.src = '';
            updateStatus(false);
        }

        function toggleFullscreen() {
            if (!isFullscreen) {
                screen.classList.add('fullscreen');
                isFullscreen = true;
                document.querySelectorAll('#container > *:not(#screen-container)').forEach(el => {
                    el.style.display = 'none';
                });
            } else {
                screen.classList.remove('fullscreen');
                isFullscreen = false;
                document.querySelectorAll('#container > *:not(#screen-container)').forEach(el => {
                    el.style.display = '';
                });
            }
        }

        function changeScreen(screenIndex) {
            currentScreen = screenIndex;
            
            document.querySelectorAll('.screen-btn').forEach(btn => {
                btn.classList.remove('active');
            });
            document.querySelector('[data-screen="' + screenIndex + '"]').classList.add('active');
            
            const screenName = screenIndex === 'all' ? 'All Screens' : 'Screen ' + (parseInt(screenIndex) + 1);
            document.getElementById('current-screen').textContent = 'Current: ' + screenName;
            
            if (ws && ws.readyState === WebSocket.OPEN) {
                ws.send('screen:' + screenIndex);
                console.log('🖥️ Screen changé vers:', screenIndex);
            }
        }

        function setFPS(fps) {
            currentFPS = fps;
            
            document.querySelectorAll('.fps-btn').forEach(btn => {
                btn.classList.remove('active');
            });
            document.querySelector('[data-fps="' + fps + '"]').classList.add('active');
            
            if (ws && ws.readyState === WebSocket.OPEN) {
                ws.send('fps:' + fps);
                console.log('🚀 FPS changé vers:', fps);
                
                // Reset stats pour mesure immédiate
                frameCount = 0;
                lastFrameTime = Date.now();
                fpsDisplay = 0;
                document.getElementById('fps-info').textContent = 'FPS: ' + fps + ' (mesure...)';
            } else {
                document.getElementById('fps-info').textContent = 'FPS: ' + fps;
            }
        }

        function updateImageInfo() {
            if (screen.naturalWidth && screen.naturalHeight) {
                document.getElementById('resolution').textContent = 
                    'Resolution: ' + screen.naturalWidth + 'x' + screen.naturalHeight;
            }
        }

        // Événements clavier
        document.addEventListener('keydown', function(e) {
            if (e.key === 'Escape' && isFullscreen) {
                toggleFullscreen();
            } else if (e.key === 'f' || e.key === 'F') {
                toggleFullscreen();
            } else if (e.key >= '1' && e.key <= '4') {
                const screenIndex = e.key === '4' ? 'all' : parseInt(e.key) - 1;
                changeScreen(screenIndex);
            }
        });

        // Double-clic pour plein écran
        screen.ondblclick = toggleFullscreen;

        // Clic simple pour rafraîchir
        screen.onclick = function(e) {
            if (!isFullscreen && ws && ws.readyState === WebSocket.OPEN) {
                ws.send('refresh');
            }
        };

        // Détecter le chargement de l'image
        screen.onload = updateImageInfo;

        // Connexion automatique au chargement
        window.onload = function() {
            updateStatus(false);
            connect();
        };
    </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, html)
}

func main() {
	// Vérifier les écrans disponibles
	numScreens := screenshot.NumActiveDisplays()
	switch runtime.GOOS {
	case "windows":
		fmt.Printf("✅ Windows détecté - %d écran(s)\n", numScreens)
	case "linux":
		fmt.Printf("✅ Linux détecté - %d écran(s)\n", numScreens)
	case "darwin":
		fmt.Printf("✅ macOS détecté - %d écran(s)\n", numScreens)
	default:
		fmt.Printf("OS non supporté: %s\n", runtime.GOOS)
		os.Exit(1)
	}

	if numScreens == 0 {
		fmt.Println("Aucun écran détecté!")
		os.Exit(1)
	}

	// Afficher les informations des écrans
	for i := 0; i < numScreens; i++ {
		bounds := screenshot.GetDisplayBounds(i)
		fmt.Printf("   Écran %d: %dx%d à (%d,%d)\n", i, bounds.Dx(), bounds.Dy(), bounds.Min.X, bounds.Min.Y)
	}

	streamer := NewScreenStreamer()

	// Routes HTTP
	http.HandleFunc("/", serveHTML)
	http.HandleFunc("/ws", streamer.handleWebSocket)

	// Démarrer le streaming en arrière-plan
	go streamer.startStreaming()

	port := "8080"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	fmt.Printf("🚀 Serveur démarré sur http://localhost:%s\n", port)
	fmt.Printf("📺 Interface web optimisée avec capture native!\n")
	fmt.Printf("⚡ Support jusqu'à 120 FPS avec diffusion binaire\n")

	log.Fatal(http.ListenAndServe(":"+port, nil))
	// ==============> TODO 3
	// Ajout HTTPS avec certificats TLS
	// log.Fatal(http.ListenAndServeTLS(":8443", "cert.pem", "key.pem", nil))
}
