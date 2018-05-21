package pigo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"unsafe"
)

type pigo struct {
	treeDepth     uint32
	treeNum       uint32
	treeCodes     []int8
	treePred      []float32
	treeThreshold []float32
}

func NewPigo() *pigo {
	return &pigo{}
}

// Unpack unpack the binary face classification file.
func (pg *pigo) Unpack(packet []byte) *pigo {
	var (
		treeDepth     uint32
		treeNum       uint32
		treeCodes     []int8
		treePred      []float32
		treeThreshold []float32
	)

	// We skip the first 8 bytes of the cascade file.
	pos := 8
	buff := make([]byte, 4)
	dataView := bytes.NewBuffer(buff)

	// Read the depth (size) of each tree and write it into the buffer array.
	_, err := dataView.Write([]byte{packet[pos+0], packet[pos+1], packet[pos+2], packet[pos+3]})
	if err != nil {
		log.Fatalf("Error writing tree size into the buffer array: %v", err)
	}

	if dataView.Len() > 0 {
		treeDepth = binary.LittleEndian.Uint32(packet[pos:])
		fmt.Println("Tree depth: ", treeDepth)
		pos += 4

		// Get the number of cascade trees as 32-bit unsigned integer and write it into the buffer array.
		_, err := dataView.Write([]byte{packet[pos+0], packet[pos+1], packet[pos+2], packet[pos+3]})
		if err != nil {
			log.Fatalf("Error writing cascade trees into the buffer array: %v", err)
		}

		treeNum = binary.LittleEndian.Uint32(packet[pos:])
		pos += 4
		fmt.Println("Tree numbers: ", treeNum)

		for t := 0; t < int(treeNum); t++ {
			treeCodes = append(treeCodes, []int8{0, 0, 0, 0}...)

			code := packet[pos : pos+int(4*math.Pow(2, float64(treeDepth))-4)]
			signedCode := *(*[]int8)(unsafe.Pointer(&code))
			treeCodes = append(treeCodes, signedCode...)

			pos = pos + int(4*math.Pow(2, float64(treeDepth))-4)

			// Read prediction from tree's leaf nodes.
			for i := 0; i < int(math.Pow(2, float64(treeDepth))); i++ {
				_, err := dataView.Write([]byte{packet[pos+0], packet[pos+1], packet[pos+2], packet[pos+3]})
				if err != nil {
					log.Fatalf("Error writing leaf node predictions into the buffer array: %v", err)
				}
				u32pred := binary.LittleEndian.Uint32(packet[pos:])
				// Convert uint32 to float32
				f32pred := *(*float32)(unsafe.Pointer(&u32pred))
				treePred = append(treePred, f32pred)
				pos += 4
			}

			// Read tree nodes threshold values.
			_, err := dataView.Write([]byte{packet[pos+0], packet[pos+1], packet[pos+2], packet[pos+3]})
			if err != nil {
				log.Fatalf("Error writing tree nodes threshold value into the buffer array: %v", err)
			}
			u32thr := binary.LittleEndian.Uint32(packet[pos:])
			// Convert uint32 to float32
			f32thr := *(*float32)(unsafe.Pointer(&u32thr))
			treeThreshold = append(treeThreshold, f32thr)
			pos += 4
		}
	}

	fmt.Println("Codes:", len(treeCodes))
	fmt.Println("Preds:", len(treePred))
	fmt.Println("Thresh:", len(treeThreshold))
	fmt.Println("Depth:", treeDepth)
	fmt.Println("N trees:", treeNum)

	return &pigo{
		treeDepth,
		treeNum,
		treeCodes,
		treePred,
		treeThreshold,
	}
}

// classifyRegion constructs the classification function based on the parsed binary data.
func (pg *pigo) classifyRegion(r, c, s int, pixels []uint8, dim int) float32 {
	var (
		root  int = 0
		out   float32
		pTree = int(math.Pow(2, float64(pg.treeDepth)))
	)

	r = r * 256
	c = c * 256

	for i := 0; i < int(pg.treeNum); i++ {
		var idx = 1

		for j := 0; j < int(pg.treeDepth); j++ {
			var pix = 0
			var x1 = ((r+int(pg.treeCodes[root + 4*idx + 0])*s) >> 8)*dim+((c+int(pg.treeCodes[root + 4*idx + 1])*s) >> 8)
			var x2 = ((r+int(pg.treeCodes[root + 4*idx + 2])*s) >> 8)*dim+((c+int(pg.treeCodes[root + 4*idx + 3])*s) >> 8)

			var px1 = pixels[x1]
			var px2 = pixels[x2]

			if (px1 <= px2) {
				pix = 1
			} else {
				pix = 0
			}
			idx = 2*idx + pix
		}
		out += pg.treePred[pTree*i+idx-pTree]

		if out <= pg.treeThreshold[i] {
			return -1.0
		} else {
			root += 4 * pTree
			//fmt.Println(root)
		}
	}

	return 1.0
}

type CascadeParams struct {
	MinSize     int
	MaxSize     int
	ShiftFactor float64
	ScaleFactor float64
}

type ImageParams struct {
	Pixels []uint8
	Rows int
	Cols int
	Dim  int
}

type detection struct {
	row    int
	col    int
	center int
	q      float32
}

func (pg *pigo) RunCascade(img ImageParams, opts CascadeParams) []detection {
	var detections []detection
	var pixels = img.Pixels

	scale := opts.MinSize

	// Run the classification function over the detection window
	// and check if the false positive rate is above a certain value.
	for scale <= opts.MaxSize {
		step := int(math.Max(opts.ShiftFactor * float64(scale), 1))
		offset := (scale /2 + 1)

		for row := offset; row <= img.Rows-offset; row += step {
			for col := offset; col <= img.Cols-offset; col += step {
				q := pg.classifyRegion(row, col, scale, pixels, img.Dim)
				if q > 0.0 {
					detections = append(detections, detection{row, col, scale, q})
				}
			}
		}
		scale = int(float64(scale) * opts.ScaleFactor)
		//fmt.Println(scale)
	}
	return detections
}