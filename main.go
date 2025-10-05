package main

import (
	"image/color"
	"log"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/solarlune/resolv"
)

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
	return g
}

func (g *Game) Update() error {
	if g.gameOver {
		// Press R to restart
		if ebiten.IsKeyPressed(ebiten.KeyR) {
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

func aabb(a rect, b rect) bool {

	return a.X < b.X+b.W &&
		a.X+a.W > b.X &&
		a.Y < b.Y+b.H &&
		a.Y+a.H > b.Y
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
			if aabb(g.bullets[bi], g.enemies[ei]) {
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
	ebitenutil.DrawRect(screen, g.player.X, g.player.Y, g.player.W, g.player.H, color.RGBA{80, 200, 255, 255})

	// bullets
	for _, b := range g.bullets {
		ebitenutil.DrawRect(screen, b.X, b.Y, b.W, b.H, color.RGBA{255, 240, 120, 255})
	}

	// enemies
	for _, e := range g.enemies {
		ebitenutil.DrawRect(screen, e.X, e.Y, e.W, e.H, color.RGBA{255, 80, 120, 255})
	}

	// HUD
	//	ebitenubugPrint(screen, fmt.Sprintf("Score: %d | Lives: %d\nSpace: shoot | Arrows/A/D: move | R: restart", g.score, g.lives))

	if g.gameOver {
		overlay := color.RGBA{0, 0, 0, 180}
		ebitenutil.DrawRect(screen, 0, 0, screenW, screenH, overlay)
		ebitenutil.DebugPrintAt(screen, "GAME OVER\nPress R to restart", screenW/2-60, screenH/2-10)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenW, screenH
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
