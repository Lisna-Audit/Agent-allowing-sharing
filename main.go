package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kbinani/screenshot"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // En production, sécuriser ça
	},
}

type ControlEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
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
		currentScreen: -1,
		currentFPS:    10,
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

func simulateMouseClick(x, y int, button string, action string) error {
	switch runtime.GOOS {
	case "windows":
		return simulateMouseWindows(x, y, button, action)
	case "linux":
		return simulateMouseLinux(x, y, button, action)
	case "darwin":
		return simulateMouseMacOS(x, y, button, action)
	default:
		return fmt.Errorf("OS non supporté: %s", runtime.GOOS)
	}
}

func simulateMouseWindows(x, y int, button string, action string) error {
	var cmd *exec.Cmd

	if action == "move" {
		psScript := fmt.Sprintf(`
		Add-Type -AssemblyName System.Windows.Forms
		[System.Windows.Forms.Cursor]::Position = New-Object System.Drawing.Point(%d, %d)
		`, x, y)
		cmd = exec.Command("powershell", "-WindowStyle", "Hidden", "-Command", psScript)
	} else if action == "down" || action == "up" {
		buttonCode := "0x0002"
		if button == "right" {
			buttonCode = "0x0008"
		}
		if action == "up" {
			if button == "left" {
				buttonCode = "0x0004"
			} else if button == "right" {
				buttonCode = "0x0010"
			}
		}

		psScript := fmt.Sprintf(`
		Add-Type -AssemblyName System.Windows.Forms
		[System.Windows.Forms.Cursor]::Position = New-Object System.Drawing.Point(%d, %d)
		Add-Type @'
		using System;
		using System.Runtime.InteropServices;
		public class Win32 {
			[DllImport("user32.dll")]
			public static extern void mouse_event(uint dwFlags, uint dx, uint dy, uint dwData, UIntPtr dwExtraInfo);
		}
		'@
		[Win32]::mouse_event(%s, %d, %d, 0, 0)
		`, x, y, buttonCode, x, y)
		cmd = exec.Command("powershell", "-WindowStyle", "Hidden", "-Command", psScript)
	}

	if cmd != nil {
		return cmd.Run()
	}
	return nil
}

func simulateMouseLinux(x, y int, button string, action string) error {
	if action == "move" {
		cmd := exec.Command("xdotool", "mousemove", strconv.Itoa(x), strconv.Itoa(y))
		return cmd.Run()
	} else if action == "down" {
		buttonNum := "1"
		if button == "right" {
			buttonNum = "3"
		} else if button == "middle" {
			buttonNum = "2"
		}
		cmd := exec.Command("xdotool", "mousemove", strconv.Itoa(x), strconv.Itoa(y), "mousedown", buttonNum)
		return cmd.Run()
	} else if action == "up" {
		buttonNum := "1"
		if button == "right" {
			buttonNum = "3"
		} else if button == "middle" {
			buttonNum = "2"
		}
		cmd := exec.Command("xdotool", "mouseup", buttonNum)
		return cmd.Run()
	}
	return nil
}

func simulateMouseMacOS(x, y int, button string, action string) error {
	if action == "move" {
		script := fmt.Sprintf(`tell application "System Events" to set the mouse location to {%d, %d}`, x, y)
		cmd := exec.Command("osascript", "-e", script)
		return cmd.Run()
	} else if action == "down" || action == "up" {
		clickType := "left click"
		if button == "right" {
			clickType = "right click"
		}
		script := fmt.Sprintf(`tell application "System Events" to %s at {%d, %d}`, clickType, x, y)
		cmd := exec.Command("osascript", "-e", script)
		return cmd.Run()
	}
	return nil
}

func simulateKeyboard(key string, action string, ctrl, alt, shift bool) error {
	switch runtime.GOOS {
	case "windows":
		return simulateKeyboardWindows(key, action, ctrl, alt, shift)
	case "linux":
		return simulateKeyboardLinux(key, action, ctrl, alt, shift)
	case "darwin":
		return simulateKeyboardMacOS(key, action, ctrl, alt, shift)
	default:
		return fmt.Errorf("OS non supporté: %s", runtime.GOOS)
	}
}

func simulateKeyboardWindows(key string, action string, ctrl, alt, shift bool) error {
	winKey := key
	switch key {
	case "Enter":
		winKey = "{ENTER}"
	case "Space":
		winKey = " "
	case "Backspace":
		winKey = "{BACKSPACE}"
	case "Tab":
		winKey = "{TAB}"
	case "Escape":
		winKey = "{ESC}"
	}

	modifiers := ""
	if ctrl {
		modifiers += "^"
	}
	if alt {
		modifiers += "%"
	}
	if shift {
		modifiers += "+"
	}

	psScript := fmt.Sprintf(`
	Add-Type -AssemblyName System.Windows.Forms
	[System.Windows.Forms.SendKeys]::SendWait('%s%s')
	`, modifiers, winKey)

	cmd := exec.Command("powershell", "-WindowStyle", "Hidden", "-Command", psScript)
	return cmd.Run()
}

// Ajuster les coordonnées selon l'écran sélectionné
func adjustMouseCoordinates(screenIndex int, x, y int) (int, int) {
	if screenIndex == -1 {
		// Mode "tous les écrans" - pas d'ajustement nécessaire
		return x, y
	}

	if screenIndex >= screenshot.NumActiveDisplays() {
		// Écran invalide, retourner tel quel
		return x, y
	}

	// Obtenir les bounds de l'écran sélectionné
	bounds := screenshot.GetDisplayBounds(screenIndex)

	// Ajuster les coordonnées avec l'offset de l'écran
	adjustedX := bounds.Min.X + x
	adjustedY := bounds.Min.Y + y

	log.Printf("Coord adjustment: screen %d, original (%d,%d) -> adjusted (%d,%d)",
		screenIndex, x, y, adjustedX, adjustedY)

	return adjustedX, adjustedY
}

func simulateKeyboardLinux(key string, action string, ctrl, alt, shift bool) error {
	args := []string{}

	if action == "down" {
		args = append(args, "keydown")
	} else if action == "up" {
		args = append(args, "keyup")
	} else {
		args = append(args, "key")
	}

	keyCombo := ""
	if ctrl {
		keyCombo += "ctrl+"
	}
	if alt {
		keyCombo += "alt+"
	}
	if shift {
		keyCombo += "shift+"
	}

	xKey := strings.ToLower(key)
	switch key {
	case " ":
		xKey = "space"
	case "Enter":
		xKey = "Return"
	case "Backspace":
		xKey = "BackSpace"
	}

	keyCombo += xKey
	args = append(args, keyCombo)

	cmd := exec.Command("xdotool", args...)
	return cmd.Run()
}

func simulateKeyboardMacOS(key string, action string, ctrl, alt, shift bool) error {
	modifiers := ""
	if ctrl {
		modifiers += "control down, "
	}
	if alt {
		modifiers += "option down, "
	}
	if shift {
		modifiers += "shift down, "
	}

	macKey := key
	switch key {
	case "Enter":
		macKey = "return"
	case " ":
		macKey = "space"
	case "Backspace":
		macKey = "delete"
	}

	script := fmt.Sprintf(`tell application "System Events" to key code (key code of "%s") using {%s}`, macKey, strings.TrimSuffix(modifiers, ", "))
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Run()
}

func captureScreen(screenIndex int) (image.Image, error) {
	var img *image.RGBA
	var err error

	if screenIndex == -1 {
		if screenshot.NumActiveDisplays() == 0 {
			return nil, fmt.Errorf("aucun écran détecté")
		}

		bounds := screenshot.GetDisplayBounds(0)
		for i := 1; i < screenshot.NumActiveDisplays(); i++ {
			bounds = bounds.Union(screenshot.GetDisplayBounds(i))
		}
		img, err = screenshot.CaptureRect(bounds)
	} else {
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

func (s *ScreenStreamer) broadcastImage(img image.Image, quality int) error {
	var buf bytes.Buffer

	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	if err != nil {
		return fmt.Errorf("erreur encodage: %v", err)
	}

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

func (s *ScreenStreamer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Erreur upgrade WebSocket: %v", err)
		return
	}

	s.addClient(conn)

	go func() {
		defer s.removeClient(conn)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				break
			}

			command := strings.TrimSpace(string(message))

			if strings.HasPrefix(command, "{") {
				var controlEvent ControlEvent
				if err := json.Unmarshal(message, &controlEvent); err == nil {
					switch controlEvent.Type {
					case "mouse":
						if mouseData, ok := controlEvent.Data.(map[string]interface{}); ok {
							x := int(mouseData["x"].(float64))
							y := int(mouseData["y"].(float64))
							button := mouseData["button"].(string)
							action := mouseData["action"].(string)

							// Ajuster les coordonnées selon l'écran sélectionné
							adjustedX, adjustedY := adjustMouseCoordinates(s.currentScreen, x, y)

							err := simulateMouseClick(adjustedX, adjustedY, button, action)
							if err != nil {
								log.Printf("Erreur souris: %v", err)
							}
						}
					case "keyboard":
						if keyData, ok := controlEvent.Data.(map[string]interface{}); ok {
							key := keyData["key"].(string)
							action := keyData["action"].(string)
							ctrl := keyData["ctrl"].(bool)
							alt := keyData["alt"].(bool)
							shift := keyData["shift"].(bool)

							err := simulateKeyboard(key, action, ctrl, alt, shift)
							if err != nil {
								log.Printf("Erreur clavier: %v", err)
							}
						}
					}
				}
				continue
			}

			switch {
			case command == "refresh":
				continue
			case strings.HasPrefix(command, "screen:"):
				screenStr := strings.TrimPrefix(command, "screen:")
				if screenStr == "all" {
					s.currentScreen = -1
				} else {
					var screenIndex int
					if n, _ := fmt.Sscanf(screenStr, "%d", &screenIndex); n == 1 {
						if screenIndex < screenshot.NumActiveDisplays() {
							s.currentScreen = screenIndex
						}
					}
				}
			case strings.HasPrefix(command, "fps:"):
				fpsStr := strings.TrimPrefix(command, "fps:")
				var fps int
				if n, _ := fmt.Sscanf(fpsStr, "%d", &fps); n == 1 && fps > 0 && fps <= 120 {
					s.currentFPS = fps
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

func (s *ScreenStreamer) startStreaming() {
	currentFPS := s.currentFPS
	ticker := time.NewTicker(time.Second / time.Duration(currentFPS))
	defer ticker.Stop()

	log.Printf("Streaming démarré à %d FPS", currentFPS)

	frameCount := 0
	lastStatsTime := time.Now()

	for {
		select {
		case <-ticker.C:
			if len(s.clients) == 0 {
				continue
			}

			frameStart := time.Now()

			img, err := captureScreen(s.currentScreen)
			if err != nil {
				log.Printf("Erreur capture: %v", err)
				continue
			}
			captureTime := time.Since(frameStart)

			quality := 70
			if currentFPS >= 60 {
				quality = 45
			} else if currentFPS >= 30 {
				quality = 55
			} else if currentFPS <= 5 {
				quality = 90
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

			if time.Since(lastStatsTime) > 5*time.Second {
				actualFPS := float64(frameCount) / time.Since(lastStatsTime).Seconds()
				log.Printf("FPS cible: %d | FPS réel: %.1f | Capture: %dms | Encode+Send: %dms | Total: %dms",
					currentFPS, actualFPS, captureTime.Milliseconds(), encodeTime.Milliseconds(), frameTotal.Milliseconds())
				frameCount = 0
				lastStatsTime = time.Now()
			}

		case newFPS := <-s.fpsChanged:
			if newFPS != currentFPS {
				oldFPS := currentFPS
				currentFPS = newFPS
				ticker.Stop()
				ticker = time.NewTicker(time.Second / time.Duration(currentFPS))
				log.Printf("FPS changé: %d -> %d", oldFPS, currentFPS)
				frameCount = 0
				lastStatsTime = time.Now()
			}
		}
	}
}

func serveHTML(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>VM Desktop Viewer</title>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        * { box-sizing: border-box; }
        body { margin: 0; padding: 10px; background: #1a1a1a; color: white; font-family: 'Segoe UI', sans-serif; height: 100vh; overflow-x: hidden; }
        #container { text-align: center; height: 100%; display: flex; flex-direction: column; }
        h1 { margin: 10px 0; font-size: 1.5em; }
        #controls { margin: 10px 0; display: flex; flex-wrap: wrap; justify-content: center; gap: 10px; }
        button { background: #333; color: white; border: none; padding: 8px 16px; cursor: pointer; border-radius: 6px; font-size: 14px; transition: background 0.2s; }
        button:hover { background: #555; }
        button.active { background: #4CAF50; }
        button:disabled { background: #666; opacity: 0.5; cursor: not-allowed; }
        .control-btn.enabled { background: #FF5722; }
        .control-indicator { position: fixed; top: 10px; right: 10px; background: #FF5722; color: white; padding: 5px 10px; border-radius: 4px; font-size: 12px; display: none; z-index: 1001; }
        #status { margin: 10px 0; padding: 5px 10px; border-radius: 4px; display: inline-block; font-weight: bold; }
        .connected { background: #4CAF50; color: white; }
        .disconnected { background: #f44336; color: white; }
        #screen-container { flex: 1; display: flex; justify-content: center; align-items: center; padding: 10px; min-height: 0; }
        #screen { max-width: 100%; max-height: 100%; width: auto; height: auto; border: 2px solid #333; border-radius: 8px; cursor: pointer; transition: transform 0.1s; object-fit: contain; }
        #screen:hover { transform: scale(1.01); border-color: #4CAF50; }
        #screen.fullscreen { position: fixed; top: 0; left: 0; width: 100vw !important; height: 100vh !important; max-width: 100vw; max-height: 100vh; z-index: 1000; border: none; border-radius: 0; background: black; }
        #info { font-size: 12px; color: #aaa; margin: 5px 0; }
        .screen-selector, .fps-selector { display: flex; gap: 5px; align-items: center; }
        @media (max-width: 768px) { body { padding: 5px; } h1 { font-size: 1.2em; margin: 5px 0; } button { padding: 6px 12px; font-size: 12px; } #controls { gap: 5px; } }
    </style>
</head>
<body>
    <div id="container">
        <h1>VM Desktop Viewer with Remote Control</h1>
        <div id="status" class="disconnected">Disconnected</div>
        <div id="controls">
            <button id="connectBtn" onclick="connect()">Connect</button>
            <button id="disconnectBtn" onclick="disconnect()">Disconnect</button>
            <button onclick="toggleFullscreen()">Fullscreen</button>
            <button id="controlBtn" onclick="toggleControl()" class="control-btn">Enable Control</button>
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
            <span id="current-screen">Current: All Screens</span> |
            <span id="control-status">Control: Disabled</span>
        </div>
        <div id="screen-container">
            <img id="screen" alt="VM Desktop" />
        </div>
        <div id="control-indicator" class="control-indicator">REMOTE CONTROL ACTIVE</div>
    </div>
    <script>
        let ws = null, currentScreen = 'all', currentFPS = 10, isFullscreen = false, controlEnabled = false;
        let frameCount = 0, lastFrameTime = 0, fpsDisplay = 0;
        const screen = document.getElementById('screen'), status = document.getElementById('status');
        const connectBtn = document.getElementById('connectBtn'), disconnectBtn = document.getElementById('disconnectBtn');
        const controlBtn = document.getElementById('controlBtn');
        const controlIndicator = document.getElementById('control-indicator');

        function updateStatus(connected) {
            if (connected) { status.textContent = 'Connected'; status.className = 'connected'; connectBtn.disabled = true; disconnectBtn.disabled = false; }
            else { status.textContent = 'Disconnected'; status.className = 'disconnected'; connectBtn.disabled = false; disconnectBtn.disabled = true; }
        }

        function connect() {
            if (ws && ws.readyState === WebSocket.OPEN) return;
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            ws = new WebSocket(protocol + '//' + window.location.host + '/ws');
            ws.binaryType = 'arraybuffer';
            
            ws.onopen = function() {
                updateStatus(true);
                if (ws.readyState === WebSocket.OPEN) { ws.send('screen:' + currentScreen); ws.send('fps:' + currentFPS); }
                frameCount = 0; lastFrameTime = Date.now();
            };
            
            ws.onmessage = function(event) {
                const blob = new Blob([event.data], {type: 'image/jpeg'});
                const imageUrl = URL.createObjectURL(blob);
                if (screen.src.startsWith('blob:')) URL.revokeObjectURL(screen.src);
                screen.src = imageUrl;
                
                const now = Date.now(); frameCount++;
                if (now - lastFrameTime >= 2000) {
                    fpsDisplay = Math.round(frameCount / ((now - lastFrameTime) / 1000));
                    document.getElementById('fps-info').textContent = 'FPS: ' + currentFPS + ' (real: ' + fpsDisplay + ')';
                    frameCount = 0; lastFrameTime = now;
                }
                updateImageInfo();
            };
            
            ws.onclose = function() { updateStatus(false); setTimeout(connect, 2000); };
            ws.onerror = function(error) { console.error('WebSocket error:', error); updateStatus(false); };
        }

        function disconnect() {
            if (ws) { ws.close(); ws = null; }
            if (screen.src.startsWith('blob:')) URL.revokeObjectURL(screen.src);
            screen.src = ''; updateStatus(false);
        }

        function toggleFullscreen() {
            if (!isFullscreen) {
                screen.classList.add('fullscreen'); isFullscreen = true;
                document.querySelectorAll('#container > *:not(#screen-container)').forEach(el => el.style.display = 'none');
            } else {
                screen.classList.remove('fullscreen'); isFullscreen = false;
                document.querySelectorAll('#container > *:not(#screen-container)').forEach(el => el.style.display = '');
            }
        }

        function changeScreen(screenIndex) {
            currentScreen = screenIndex;
            document.querySelectorAll('.screen-btn').forEach(btn => btn.classList.remove('active'));
            document.querySelector('[data-screen="' + screenIndex + '"]').classList.add('active');
            const screenName = screenIndex === 'all' ? 'All Screens' : 'Screen ' + (parseInt(screenIndex) + 1);
            document.getElementById('current-screen').textContent = 'Current: ' + screenName;
            if (ws && ws.readyState === WebSocket.OPEN) ws.send('screen:' + screenIndex);
        }

        function setFPS(fps) {
            currentFPS = fps;
            document.querySelectorAll('.fps-btn').forEach(btn => btn.classList.remove('active'));
            document.querySelector('[data-fps="' + fps + '"]').classList.add('active');
            if (ws && ws.readyState === WebSocket.OPEN) {
                ws.send('fps:' + fps);
                frameCount = 0; lastFrameTime = Date.now(); fpsDisplay = 0;
                document.getElementById('fps-info').textContent = 'FPS: ' + fps + ' (measuring...)';
            } else document.getElementById('fps-info').textContent = 'FPS: ' + fps;
        }

        function toggleControl() {
            controlEnabled = !controlEnabled;
            
            if (controlEnabled) {
                controlBtn.textContent = 'Disable Control'; 
                controlBtn.classList.add('enabled');
                controlIndicator.style.display = 'block'; 
                document.getElementById('control-status').textContent = 'Control: Enabled';
                console.log('Remote control ENABLED');
            } else {
                controlBtn.textContent = 'Enable Control'; 
                controlBtn.classList.remove('enabled');
                controlIndicator.style.display = 'none'; 
                document.getElementById('control-status').textContent = 'Control: Disabled';
                console.log('Remote control DISABLED');
            }
        }

        function sendControlEvent(type, data) {
            if (!controlEnabled || !ws || ws.readyState !== WebSocket.OPEN) return;
            ws.send(JSON.stringify({type: type, data: data}));
        }

        function getImageCoordinates(e) {
            const rect = screen.getBoundingClientRect();
            const scaleX = screen.naturalWidth / rect.width;
            const scaleY = screen.naturalHeight / rect.height;
            
            return { 
                x: Math.round((e.clientX - rect.left) * scaleX), 
                y: Math.round((e.clientY - rect.top) * scaleY) 
            };
        }

        function updateImageInfo() {
            if (screen.naturalWidth && screen.naturalHeight) 
                document.getElementById('resolution').textContent = 'Resolution: ' + screen.naturalWidth + 'x' + screen.naturalHeight;
        }

        screen.addEventListener('mousedown', function(e) {
            if (!controlEnabled) return; e.preventDefault();
            const coords = getImageCoordinates(e);
            const button = e.button === 0 ? 'left' : e.button === 2 ? 'right' : 'middle';
            sendControlEvent('mouse', {x: coords.x, y: coords.y, button: button, action: 'down'});
        });

        screen.addEventListener('mouseup', function(e) {
            if (!controlEnabled) return; e.preventDefault();
            const coords = getImageCoordinates(e);
            const button = e.button === 0 ? 'left' : e.button === 2 ? 'right' : 'middle';
            sendControlEvent('mouse', {x: coords.x, y: coords.y, button: button, action: 'up'});
        });

        screen.addEventListener('mousemove', function(e) {
            if (!controlEnabled) return; 
            e.preventDefault();
            
            if (mouseLocked) {
                // En mode locked, déplacer le curseur virtuel
                updateMouseCursor(e.clientX, e.clientY);
            }
            
            const coords = getImageCoordinates(e);
            sendControlEvent('mouse', {x: coords.x, y: coords.y, button: 'none', action: 'move'});
        });

        screen.addEventListener('wheel', function(e) {
            if (!controlEnabled) return; e.preventDefault();
            const coords = getImageCoordinates(e);
            sendControlEvent('mouse', {x: coords.x, y: coords.y, button: 'wheel', action: 'scroll', scroll: e.deltaY > 0 ? -1 : 1});
        });

        screen.addEventListener('mouseleave', function(e) {
            if (mouseLocked) {
                // En mode locked, empêcher la souris de sortir en la ramenant au centre
                const rect = screen.getBoundingClientRect();
                const centerX = rect.left + rect.width / 2;
                const centerY = rect.top + rect.height / 2;
                
                // Note: On ne peut pas forcer la position de la souris réelle,
                // mais on peut ignorer les événements en dehors de l'image
                e.preventDefault();
            }
        });

        screen.addEventListener('mouseenter', function(e) {
            if (mouseLocked) {
                updateMouseCursor(e.clientX, e.clientY);
            }
        });

        document.addEventListener('keydown', function(e) {
            // Gérer les raccourcis spéciaux d'abord (même en mode contrôle)
            if (e.key === 'Escape' && isFullscreen) {
                toggleFullscreen();
                return;
            } else if (e.key === 'f' || e.key === 'F') {
                if (e.ctrlKey) {
                    toggleFullscreen();
                    return;
                }
            } else if (e.key >= '1' && e.key <= '4' && e.ctrlKey) {
                const screenIndex = e.key === '4' ? 'all' : parseInt(e.key) - 1;
                changeScreen(screenIndex);
                return;
            }
            
            // Si le contrôle est activé, envoyer les touches
            if (controlEnabled && !e.repeat) {
                e.preventDefault();
                sendControlEvent('keyboard', {key: e.key, action: 'down', ctrl: e.ctrlKey, alt: e.altKey, shift: e.shiftKey});
            }
        });

        document.addEventListener('keyup', function(e) {
            if (controlEnabled) {
                e.preventDefault();
                sendControlEvent('keyboard', {key: e.key, action: 'up', ctrl: e.ctrlKey, alt: e.altKey, shift: e.shiftKey});
            }
        });

        // Raccourcis navigation (seulement si contrôle désactivé)
        document.addEventListener('keydown', function(e) {
            if (!controlEnabled) {
                if (e.key === 'Escape' && isFullscreen) toggleFullscreen();
                else if (e.key === 'f' || e.key === 'F') toggleFullscreen();
                else if (e.key >= '1' && e.key <= '4') {
                    const screenIndex = e.key === '4' ? 'all' : parseInt(e.key) - 1;
                    changeScreen(screenIndex);
                }
            }
        });

        screen.ondblclick = function(e) { if (!controlEnabled) toggleFullscreen(); };
        screen.onclick = function(e) { 
            if (!controlEnabled && !isFullscreen && ws && ws.readyState === WebSocket.OPEN) 
                ws.send('refresh'); 
        };
        screen.onload = updateImageInfo;
        window.onload = function() { updateStatus(false); connect(); };
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, html)
}

func main() {
	numScreens := screenshot.NumActiveDisplays()
	switch runtime.GOOS {
	case "windows":
		fmt.Printf("Windows détecté - %d écran(s)\n", numScreens)
		fmt.Println("Dépendances pour contrôle : PowerShell (inclus)")
		fmt.Println("IMPORTANT: Lancez en ADMINISTRATEUR pour le contrôle souris/clavier")
		fmt.Println("  -> Clic droit sur l'exe -> 'Exécuter en tant qu'administrateur'")
		fmt.Println("  -> Ou depuis un PowerShell/CMD administrateur")
	case "linux":
		fmt.Printf("Linux détecté - %d écran(s)\n", numScreens)
		fmt.Println("Dépendances pour contrôle : sudo apt install xdotool")
		if _, err := exec.LookPath("xdotool"); err != nil {
			fmt.Println("ATTENTION: xdotool non trouvé - le contrôle ne fonctionnera pas")
			fmt.Println("Installation: sudo apt install xdotool")
		} else {
			fmt.Println("xdotool trouvé - contrôle disponible")
		}
	case "darwin":
		fmt.Printf("macOS détecté - %d écran(s)\n", numScreens)
		fmt.Println("Dépendances pour contrôle : osascript (inclus)")
		fmt.Println("ATTENTION: Accordez les permissions d'accessibilité dans Préférences Système")
	default:
		fmt.Printf("OS non supporté: %s\n", runtime.GOOS)
		os.Exit(1)
	}

	if numScreens == 0 {
		fmt.Println("Aucun écran détecté!")
		os.Exit(1)
	}

	for i := 0; i < numScreens; i++ {
		bounds := screenshot.GetDisplayBounds(i)
		fmt.Printf("   Écran %d: %dx%d à (%d,%d)\n", i, bounds.Dx(), bounds.Dy(), bounds.Min.X, bounds.Min.Y)
	}

	streamer := NewScreenStreamer()
	http.HandleFunc("/", serveHTML)
	http.HandleFunc("/ws", streamer.handleWebSocket)
	go streamer.startStreaming()

	port := "8080"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	fmt.Printf("Serveur démarré sur http://localhost:%s\n", port)
	fmt.Printf("Interface web avec streaming + contrôle à distance!\n")
	fmt.Printf("Support jusqu'à 120 FPS avec interactions souris/clavier\n")
	fmt.Printf("Cliquez 'Enable Control' pour activer le contrôle à distance\n")

	log.Fatal(http.ListenAndServe(":"+port, nil))
}
