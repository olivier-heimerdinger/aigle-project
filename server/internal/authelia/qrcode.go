// Package authelia gère la synchronisation bidirectionnelle avec Authelia :
//   - Hachage et vérification Argon2id compatibles avec users_database.yml d'Authelia
//   - Moteur TOTP RFC 6238 (SHA1, 6 chiffres, 30 secondes) + génération de QR Code SVG autonome
//   - Mise à jour dynamique des règles two_factor par projet dans configuration.yml d'Authelia
package authelia

import (
	"bytes"
	"encoding/base64"
	"fmt"
)

// GenerateQRCodeDataURI génère un QR Code ISO/IEC 18004 (Mode Byte, Niveau L, Version auto 1..10)
// sous forme d'image SVG encodée en Data URI (data:image/svg+xml;base64,...) 100% autonome sans dépendance externe.
func GenerateQRCodeDataURI(content string) string {
	matrix := encodeQRMatrix([]byte(content))
	size := len(matrix)
	if size == 0 {
		return ""
	}
	margin := 2
	total := size + margin*2

	var buf bytes.Buffer
	fmt.Fprintf(&buf, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`, total, total)
	buf.WriteString(`<rect width="100%" height="100%" fill="#ffffff"/>`)
	buf.WriteString(`<path fill="#0f172a" d="`)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if matrix[y][x] {
				fmt.Fprintf(&buf, "M%d,%dh1v1h-1z", x+margin, y+margin)
			}
		}
	}
	buf.WriteString(`"/></svg>`)
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// Paramètres ISO/IEC 18004 EC Level L (1 bloc pour versions 1..5, 2 blocs pour versions 6..9, 4 blocs pour version 10)
type qrVersionInfo struct {
	version    int
	totalBytes int
	ecPerBlock int
	blocks     int
	align      []int
}

var qrVersionsL = []qrVersionInfo{
	{1, 26, 7, 1, nil},
	{2, 44, 10, 1, []int{6, 18}},
	{3, 70, 15, 1, []int{6, 22}},
	{4, 100, 20, 1, []int{6, 26}},
	{5, 134, 26, 1, []int{6, 30}},
	{6, 172, 18, 2, []int{6, 34}},
	{7, 196, 20, 2, []int{6, 22, 38}},
	{8, 242, 24, 2, []int{6, 24, 42}},
	{9, 292, 30, 2, []int{6, 26, 46}},
	{10, 346, 18, 4, []int{6, 28, 50}},
}

func encodeQRMatrix(data []byte) [][]bool {
	var v qrVersionInfo
	for _, cand := range qrVersionsL {
		dataCap := cand.totalBytes - cand.ecPerBlock*cand.blocks
		headBytes := 2
		if cand.version >= 10 {
			headBytes = 3
		}
		if len(data)+headBytes <= dataCap {
			v = cand
			break
		}
	}
	if v.version == 0 {
		v = qrVersionsL[len(qrVersionsL)-1]
		maxLen := v.totalBytes - v.ecPerBlock*v.blocks - 3
		if len(data) > maxLen {
			data = data[:maxLen]
		}
	}

	dataCap := v.totalBytes - v.ecPerBlock*v.blocks
	var bits []uint8
	appendBits := func(val int, count int) {
		for i := count - 1; i >= 0; i-- {
			bits = append(bits, uint8((val>>i)&1))
		}
	}

	// Mode Byte (0100)
	appendBits(0b0100, 4)
	if v.version < 10 {
		appendBits(len(data), 8)
	} else {
		appendBits(len(data), 16)
	}
	for _, b := range data {
		appendBits(int(b), 8)
	}
	// Terminateur (jusqu'à 4 zéros)
	for i := 0; i < 4 && len(bits) < dataCap*8; i++ {
		bits = append(bits, 0)
	}
	for len(bits)%8 != 0 {
		bits = append(bits, 0)
	}
	codewords := make([]byte, len(bits)/8)
	for i := range codewords {
		var b byte
		for j := 0; j < 8; j++ {
			b = (b << 1) | bits[i*8+j]
		}
		codewords[i] = b
	}
	padBytes := []byte{0xEC, 0x11}
	for len(codewords) < dataCap {
		codewords = append(codewords, padBytes[len(codewords)%2])
	}

	// Découpage en blocs et calcul Reed-Solomon GF(256)
	blockSize := dataCap / v.blocks
	extra := dataCap % v.blocks
	dataBlocks := make([][]byte, v.blocks)
	ecBlocks := make([][]byte, v.blocks)
	offset := 0
	for i := 0; i < v.blocks; i++ {
		sz := blockSize
		if i >= v.blocks-extra {
			sz++
		}
		dataBlocks[i] = codewords[offset : offset+sz]
		offset += sz
		ecBlocks[i] = rsEncode(dataBlocks[i], v.ecPerBlock)
	}

	var finalBytes []byte
	maxDataLen := blockSize + 1
	for col := 0; col < maxDataLen; col++ {
		for b := 0; b < v.blocks; b++ {
			if col < len(dataBlocks[b]) {
				finalBytes = append(finalBytes, dataBlocks[b][col])
			}
		}
	}
	for col := 0; col < v.ecPerBlock; col++ {
		for b := 0; b < v.blocks; b++ {
			finalBytes = append(finalBytes, ecBlocks[b][col])
		}
	}

	dim := 17 + 4*v.version
	modules := make([][]bool, dim)
	reserved := make([][]bool, dim)
	for i := range modules {
		modules[i] = make([]bool, dim)
		reserved[i] = make([]bool, dim)
	}

	setFunction := func(x, y int, dark bool) {
		if x >= 0 && x < dim && y >= 0 && y < dim {
			modules[y][x] = dark
			reserved[y][x] = true
		}
	}

	drawFinder := func(cx, cy int) {
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				dist := absInt(dx)
				if absInt(dy) > dist {
					dist = absInt(dy)
				}
				setFunction(cx+dx, cy+dy, dist != 2 && dist != 4)
			}
		}
	}
	drawFinder(3, 3)
	drawFinder(dim-4, 3)
	drawFinder(3, dim-4)

	for i := 0; i < dim; i++ {
		if !reserved[6][i] {
			setFunction(i, 6, i%2 == 0)
		}
		if !reserved[i][6] {
			setFunction(6, i, i%2 == 0)
		}
	}

	for _, ay := range v.align {
		for _, ax := range v.align {
			if (ax == 6 && ay == 6) || (ax == 6 && ay == dim-7) || (ax == dim-7 && ay == 6) {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					dist := absInt(dx)
					if absInt(dy) > dist {
						dist = absInt(dy)
					}
					setFunction(ax+dx, ay+dy, dist != 1)
				}
			}
		}
	}

	// Réservation des zones de format + Dark Module
	for i := 0; i < 9; i++ {
		if i != 6 {
			reserved[8][i] = true
			reserved[i][8] = true
		}
	}
	for i := 0; i < 8; i++ {
		reserved[8][dim-1-i] = true
		reserved[dim-1-i][8] = true
	}
	setFunction(8, dim-8, true)

	if v.version >= 7 {
		vInfo := qrVersionBits(v.version)
		for i := 0; i < 18; i++ {
			bit := ((vInfo >> i) & 1) != 0
			a := dim - 11 + (i % 3)
			b := i / 3
			setFunction(b, a, bit)
			setFunction(a, b, bit)
		}
	}

	// Placement des bits de données en zigzag et application du masque 0 ((x+y)%2 == 0)
	bitIdx := 0
	totalBits := len(finalBytes) * 8
	up := true
	for right := dim - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := 0; vert < dim; vert++ {
			y := vert
			if up {
				y = dim - 1 - vert
			}
			for j := 0; j < 2; j++ {
				x := right - j
				if !reserved[y][x] {
					var dark bool
					if bitIdx < totalBits {
						dark = ((finalBytes[bitIdx/8] >> (7 - (bitIdx % 8))) & 1) != 0
						bitIdx++
					}
					if (x+y)%2 == 0 {
						dark = !dark
					}
					modules[y][x] = dark
				}
			}
		}
		up = !up
	}

	// Écriture des 15 bits de format pour EC Level L (01) + Masque 0 (000) => 0x77C4
	formatBits := 0x77C4
	for i := 0; i <= 5; i++ {
		setFunction(8, i, ((formatBits>>i)&1) != 0)
	}
	setFunction(8, 7, ((formatBits>>6)&1) != 0)
	setFunction(8, 8, ((formatBits>>7)&1) != 0)
	setFunction(7, 8, ((formatBits>>8)&1) != 0)
	for i := 9; i < 15; i++ {
		setFunction(14-i, 8, ((formatBits>>i)&1) != 0)
	}
	for i := 0; i < 8; i++ {
		setFunction(dim-1-i, 8, ((formatBits>>i)&1) != 0)
	}
	for i := 8; i < 15; i++ {
		setFunction(8, dim-15+i, ((formatBits>>i)&1) != 0)
	}

	return modules
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func qrVersionBits(ver int) int {
	rem := ver
	for i := 0; i < 12; i++ {
		rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
	}
	return (ver << 12) | rem
}

func gfMul(x, y byte) byte {
	var z byte
	for i := 7; i >= 0; i-- {
		z = (z << 1) ^ ((z >> 7) * 0x1D)
		if ((y >> i) & 1) != 0 {
			z ^= x
		}
	}
	return z
}

func rsEncode(data []byte, degree int) []byte {
	gen := make([]byte, degree)
	gen[degree-1] = 1
	root := byte(1)
	for i := 0; i < degree; i++ {
		for j := 0; j < degree; j++ {
			gen[j] = gfMul(gen[j], root)
			if j+1 < degree {
				gen[j] ^= gen[j+1]
			}
		}
		root = gfMul(root, 0x02)
	}
	res := make([]byte, degree)
	for _, b := range data {
		factor := b ^ res[0]
		copy(res, res[1:])
		res[degree-1] = 0
		for j := 0; j < degree; j++ {
			res[j] ^= gfMul(gen[j], factor)
		}
	}
	return res
}

