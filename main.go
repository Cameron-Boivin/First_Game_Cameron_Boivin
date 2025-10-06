package main

import (
	"bytes"
	"fmt"
	"image/color"
	"io"
	"log"
	"math/rand/v2"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/mp3"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/solarlune/resolv"
)

var sharedAudioContext *audio.Context

const (
	screenW       = 480
	screenH       = 640
	playerW       = 32
	playerH       = 20
	playerSpeed   = 4
	bulletW       = 4
	bulletH       = 8
	bulletSpeed   = 8
	enemyW        = 28
	enemyH        = 18
	enemySpeed    = 2
	spawnEvery    = 30 // frames
	shootCooldown = 8  // frames
)

type rect struct {
	Collision *resolv.ConvexPolygon
	X, Y      float64
	W, H      float64
	VX, VY    float64
	Alive     bool
}

type Game struct {
	player        rect
	bullets       []rect
	enemies       []rect
	frame         int
	score         int
	lives         int
	gameOver      bool
	lastShotFrame int
	bgScrollY     float64
	bgImg         *ebiten.Image
	Space         *resolv.Space
	audioPlayer   *audio.Player
	audioContext  *audio.Context
}

func NewGame() *Game {
	g := &Game{
		player: rect{
			X:     float64(screenW/2 - playerW/2),
			Y:     float64(screenH - 80),
			W:     playerW,
			H:     playerH,
			Alive: true,
		},
		lives: 5,
	}
	g.Space = resolv.NewSpace(screenW, screenH, 1000, 1000)
	// Load background image
	bg, _, err := ebitenutil.NewImageFromFile("spacefield_a-000.png")
	if err != nil {
		log.Fatal(err)
	}
	g.bgImg = bg
	if sharedAudioContext == nil {
		sharedAudioContext = audio.NewContext(96000)
	}
	g.audioContext = sharedAudioContext
	g.audioPlayer = LoadMP3("echoesofeternitymix.mp3", g.audioContext)
	if g.audioPlayer != nil {
		g.audioPlayer.SetVolume(0.8) // Ensure audible volume
		if err := g.audioPlayer.Rewind(); err != nil {
			log.Println("audio rewind error:", err)
		}
		g.audioPlayer.Play() // Start playing
	}
	return g
}

func (g *Game) Update() error {
	if g.gameOver {
		// Stop current audio while on game over
		if g.audioPlayer != nil {
			g.audioPlayer.Pause()
		}
		// Press R to restart
		if ebiten.IsKeyPressed(ebiten.KeyR) {
			if g.audioPlayer != nil {
				_ = g.audioPlayer.Close()
				g.audioPlayer = nil
			}
			*g = *NewGame()
		}
		return nil
	}

	g.frame++
	g.handleInput()
	g.spawnEnemies()
	g.updateBullets()
	g.updateEnemies()
	g.resolveCollisions()
	g.cleanup()

	// Scroll background
	g.bgScrollY += 1
	if g.bgScrollY > screenH {
		g.bgScrollY = 0
	}
	return nil
}

func (g *Game) handleInput() {
	if ebiten.IsKeyPressed(ebiten.KeyLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		g.player.X -= playerSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		g.player.X += playerSpeed
	}

	// clamp player to screen
	if g.player.X < 0 {
		g.player.X = 0
	}
	if g.player.X+g.player.W > screenW {
		g.player.X = screenW - g.player.W
	}

	// shooting with cooldown
	if ebiten.IsKeyPressed(ebiten.KeySpace) && g.frame-g.lastShotFrame >= shootCooldown {
		g.fire()
		g.lastShotFrame = g.frame
	}
}

func (g *Game) fire() {
	b := rect{
		X:         g.player.X + g.player.W/2 - bulletW/2,
		Y:         g.player.Y - bulletH,
		W:         bulletW,
		H:         bulletH,
		VY:        -bulletSpeed,
		Alive:     true,
		Collision: resolv.NewRectangle(g.player.X+g.player.W/2-bulletW/2, g.player.Y-bulletH, bulletW, bulletH),
	}
	g.Space.Add(b.Collision)
	g.bullets = append(g.bullets, b)
}

func (g *Game) spawnEnemies() {
	if g.frame%spawnEvery != 0 {
		return
	}
	x := float64(rand.IntN(screenW - enemyW))
	e := rect{
		X:         x,
		Y:         -float64(enemyH),
		W:         enemyW,
		H:         enemyH,
		VY:        enemySpeed + float64(rand.IntN(3))*0.5,
		Alive:     true,
		Collision: resolv.NewRectangle(x, -float64(enemyH), enemyW, enemyH),
	}
	g.Space.Add(e.Collision)
	g.enemies = append(g.enemies, e)
}

func (g *Game) updateBullets() {
	for i := range g.bullets {
		if !g.bullets[i].Alive {
			continue
		}
		g.bullets[i].Y += g.bullets[i].VY
		g.bullets[i].Collision.SetPosition(g.bullets[i].X, g.bullets[i].Y)
		if g.bullets[i].Y+g.bullets[i].H < 0 {
			g.bullets[i].Alive = false
		}
	}
}

func (g *Game) updateEnemies() {
	for i := range g.enemies {
		if !g.enemies[i].Alive {
			continue
		}
		g.enemies[i].Y += g.enemies[i].VY
		g.enemies[i].Collision.SetPosition(g.enemies[i].X, g.enemies[i].Y)
		if g.enemies[i].Y > screenH {
			g.enemies[i].Alive = false
			g.lives--
			if g.lives <= 0 {
				g.gameOver = true
			}
		}
	}
}
func collisionDetected(a rect, b rect) bool {
	if a.Collision == nil || b.Collision == nil {
		return false
	}
	return a.Collision.IsIntersecting(b.Collision)
}

func (g *Game) resolveCollisions() {
	// bullets vs enemies
	for bi := range g.bullets {
		if !g.bullets[bi].Alive {
			continue
		}
		for ei := range g.enemies {
			if !g.enemies[ei].Alive {
				continue
			}
			if collisionDetected(g.bullets[bi], g.enemies[ei]) {
				g.bullets[bi].Alive = false
				g.enemies[ei].Alive = false
				g.score += 10
				break
			}
		}
	}
	// No collision damage to player anymore
}

func (g *Game) cleanup() {
	// remove dead bullets
	nb := g.bullets[:0]
	for _, b := range g.bullets {
		if b.Alive {
			nb = append(nb, b)
		}
	}
	g.bullets = nb

	// remove dead enemies
	ne := g.enemies[:0]
	for _, e := range g.enemies {
		if e.Alive {
			ne = append(ne, e)
		}
	}
	g.enemies = ne
}

func (g *Game) Draw(screen *ebiten.Image) {
	// background image scrolling top -> bottom with wrap
	if g.bgImg != nil {
		bw := g.bgImg.Bounds().Dx()
		bh := g.bgImg.Bounds().Dy()
		sx := float64(screenW) / float64(bw)
		sy := float64(screenH) / float64(bh)

		// draw the segment above (wrapped)
		op1 := &ebiten.DrawImageOptions{}
		op1.GeoM.Scale(sx, sy)
		op1.GeoM.Translate(0, g.bgScrollY-float64(screenH))
		screen.DrawImage(g.bgImg, op1)

		// draw the current segment
		op2 := &ebiten.DrawImageOptions{}
		op2.GeoM.Scale(sx, sy)
		op2.GeoM.Translate(0, g.bgScrollY)
		screen.DrawImage(g.bgImg, op2)
	}

	// player
	vector.DrawFilledRect(screen, float32(g.player.X), float32(g.player.Y), float32(g.player.W), float32(g.player.H), color.RGBA{R: 80, G: 200, B: 255, A: 255}, false)

	// bullets
	for _, b := range g.bullets {
		vector.DrawFilledRect(screen, float32(b.X), float32(b.Y), float32(b.W), float32(b.H), color.RGBA{R: 255, G: 240, B: 120, A: 255}, false)
	}

	// enemies
	for _, e := range g.enemies {
		vector.DrawFilledRect(screen, float32(e.X), float32(e.Y), float32(e.W), float32(e.H), color.RGBA{R: 255, G: 80, B: 120, A: 255}, false)
	}

	// HUD
	ebitenutil.DebugPrint(screen, fmt.Sprintf("Score: %d | Lives: %d\nSpace: shoot | Arrows/A/D: move | R: restart", g.score, g.lives))

	if g.gameOver {
		overlay := color.RGBA{R: 0, G: 0, B: 0, A: 180}
		vector.DrawFilledRect(screen, float32(0), float32(0), float32(screenW), float32(screenH), overlay, false)
		ebitenutil.DebugPrintAt(screen, "GAME OVER\nPress R to restart", screenW/2-60, screenH/2-10)
	}
}

func (g *Game) Layout(_, _ int) (int, int) {
	return screenW, screenH
}

func LoadMP3(name string, context *audio.Context) *audio.Player {
	f, err := os.Open(name)
	if err != nil {
		fmt.Println("Error loading sound:", err)
		return nil
	}
	// Read the whole file into memory so the decoder doesn't depend on an open file handle.
	data, err := io.ReadAll(f)
	_ = f.Close()
	if err != nil {
		fmt.Println("Error reading sound file:", err)
		return nil
	}

	s, err := mp3.DecodeWithSampleRate(context.SampleRate(), bytes.NewReader(data))
	if err != nil {
		fmt.Println("Error interpreting sound file:", err)
		return nil
	}

	p, err := context.NewPlayer(s)
	if err != nil {
		fmt.Println("Couldn't create sound player:", err)
		return nil
	}
	return p
}

func main() {
	// Seed randomness for spawn variance
	// rand.Seed(uint64(time.Now().UnixNano()))

	ebiten.SetWindowSize(screenW, screenH)
	ebiten.SetWindowTitle("Top Scrolling Shooter (Go + Ebitengine)")

	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
