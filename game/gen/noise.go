package gen

import "math"

// SimplexNoise implements 2D and 3D simplex noise.
type SimplexNoise struct {
	perm [512]int
}

// NewSimplexNoise creates a simplex noise generator with the given seed.
func NewSimplexNoise(seed int64) *SimplexNoise {
	n := &SimplexNoise{}
	// Initialize permutation table from seed
	var p [256]int
	for i := range p {
		p[i] = i
	}
	// Fisher-Yates shuffle using seed
	s := uint64(seed)
	for i := 255; i > 0; i-- {
		s = s*6364136223846793005 + 1442695040888963407
		j := int(s>>33) % (i + 1)
		p[i], p[j] = p[j], p[i]
	}
	for i := 0; i < 512; i++ {
		n.perm[i] = p[i&255]
	}
	return n
}

// Noise2D returns 2D simplex noise in range [-1, 1].
func (n *SimplexNoise) Noise2D(x, y float64) float64 {
	const (
		f2 = 0.5 * (math.Sqrt2*math.Sqrt2 + math.Sqrt2 - 1) // (sqrt(3)-1)/2 ≈ 0.366
		g2 = (3 - math.Sqrt2*math.Sqrt2 - math.Sqrt2) / 6   // (3-sqrt(3))/6 ≈ 0.211
	)

	s := (x + y) * f2
	i := fastFloor(x + s)
	j := fastFloor(y + s)

	t := float64(i+j) * g2
	x0 := x - (float64(i) - t)
	y0 := y - (float64(j) - t)

	var i1, j1 int
	if x0 > y0 {
		i1, j1 = 1, 0
	} else {
		i1, j1 = 0, 1
	}

	x1 := x0 - float64(i1) + g2
	y1 := y0 - float64(j1) + g2
	x2 := x0 - 1 + 2*g2
	y2 := y0 - 1 + 2*g2

	ii := i & 255
	jj := j & 255

	var result float64

	t0 := 0.5 - x0*x0 - y0*y0
	if t0 >= 0 {
		t0 *= t0
		result += t0 * t0 * grad2(n.perm[ii+n.perm[jj]], x0, y0)
	}

	t1 := 0.5 - x1*x1 - y1*y1
	if t1 >= 0 {
		t1 *= t1
		result += t1 * t1 * grad2(n.perm[ii+i1+n.perm[jj+j1]], x1, y1)
	}

	t2 := 0.5 - x2*x2 - y2*y2
	if t2 >= 0 {
		t2 *= t2
		result += t2 * t2 * grad2(n.perm[ii+1+n.perm[jj+1]], x2, y2)
	}

	return 70.0 * result
}

// Noise3D returns 3D simplex noise in range [-1, 1].
func (n *SimplexNoise) Noise3D(x, y, z float64) float64 {
	const (
		f3 = 1.0 / 3.0
		g3 = 1.0 / 6.0
	)

	s := (x + y + z) * f3
	i := fastFloor(x + s)
	j := fastFloor(y + s)
	k := fastFloor(z + s)

	t := float64(i+j+k) * g3
	x0 := x - (float64(i) - t)
	y0 := y - (float64(j) - t)
	z0 := z - (float64(k) - t)

	var i1, j1, k1, i2, j2, k2 int
	if x0 >= y0 {
		if y0 >= z0 {
			i1, j1, k1, i2, j2, k2 = 1, 0, 0, 1, 1, 0
		} else if x0 >= z0 {
			i1, j1, k1, i2, j2, k2 = 1, 0, 0, 1, 0, 1
		} else {
			i1, j1, k1, i2, j2, k2 = 0, 0, 1, 1, 0, 1
		}
	} else {
		if y0 < z0 {
			i1, j1, k1, i2, j2, k2 = 0, 0, 1, 0, 1, 1
		} else if x0 < z0 {
			i1, j1, k1, i2, j2, k2 = 0, 1, 0, 0, 1, 1
		} else {
			i1, j1, k1, i2, j2, k2 = 0, 1, 0, 1, 1, 0
		}
	}

	x1 := x0 - float64(i1) + g3
	y1 := y0 - float64(j1) + g3
	z1 := z0 - float64(k1) + g3
	x2 := x0 - float64(i2) + 2*g3
	y2 := y0 - float64(j2) + 2*g3
	z2 := z0 - float64(k2) + 2*g3
	x3 := x0 - 1 + 3*g3
	y3 := y0 - 1 + 3*g3
	z3 := z0 - 1 + 3*g3

	ii := i & 255
	jj := j & 255
	kk := k & 255

	var result float64

	t0 := 0.6 - x0*x0 - y0*y0 - z0*z0
	if t0 >= 0 {
		t0 *= t0
		result += t0 * t0 * grad3(n.perm[ii+n.perm[jj+n.perm[kk]]], x0, y0, z0)
	}
	t1 := 0.6 - x1*x1 - y1*y1 - z1*z1
	if t1 >= 0 {
		t1 *= t1
		result += t1 * t1 * grad3(n.perm[ii+i1+n.perm[jj+j1+n.perm[kk+k1]]], x1, y1, z1)
	}
	t2 := 0.6 - x2*x2 - y2*y2 - z2*z2
	if t2 >= 0 {
		t2 *= t2
		result += t2 * t2 * grad3(n.perm[ii+i2+n.perm[jj+j2+n.perm[kk+k2]]], x2, y2, z2)
	}
	t3 := 0.6 - x3*x3 - y3*y3 - z3*z3
	if t3 >= 0 {
		t3 *= t3
		result += t3 * t3 * grad3(n.perm[ii+1+n.perm[jj+1+n.perm[kk+1]]], x3, y3, z3)
	}

	return 32.0 * result
}

// Octave2D returns multi-octave 2D simplex noise.
func (n *SimplexNoise) Octave2D(x, y float64, octaves int, lacunarity, persistence float64) float64 {
	var total, amplitude, frequency float64
	amplitude = 1
	frequency = 1
	maxVal := 0.0
	for i := 0; i < octaves; i++ {
		total += n.Noise2D(x*frequency, y*frequency) * amplitude
		maxVal += amplitude
		amplitude *= persistence
		frequency *= lacunarity
	}
	return total / maxVal
}

func fastFloor(x float64) int {
	xi := int(x)
	if x < float64(xi) {
		return xi - 1
	}
	return xi
}

var grad2Table = [12][2]float64{
	{1, 1}, {-1, 1}, {1, -1}, {-1, -1},
	{1, 0}, {-1, 0}, {0, 1}, {0, -1},
	{1, 1}, {-1, 1}, {1, -1}, {-1, -1},
}

func grad2(hash int, x, y float64) float64 {
	h := hash & 7
	g := grad2Table[h]
	return g[0]*x + g[1]*y
}

var grad3Table = [12][3]float64{
	{1, 1, 0}, {-1, 1, 0}, {1, -1, 0}, {-1, -1, 0},
	{1, 0, 1}, {-1, 0, 1}, {1, 0, -1}, {-1, 0, -1},
	{0, 1, 1}, {0, -1, 1}, {0, 1, -1}, {0, -1, -1},
}

func grad3(hash int, x, y, z float64) float64 {
	h := hash % 12
	g := grad3Table[h]
	return g[0]*x + g[1]*y + g[2]*z
}
