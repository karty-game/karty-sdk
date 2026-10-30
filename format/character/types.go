// Package character defines compiler-prepared skinned models and shared animation banks.
package character

import "errors"

const (
	Version        = 1
	Feature        = "world/characters@1"
	EntryName      = "@world/characters"
	MaxBytes       = 64 << 20
	MaxModels      = 8
	MaxBanks       = 8
	MaxNodes       = 512
	MaxJoints      = 64
	MaxVertices    = 65536
	MaxIndices     = 393216
	MaxClips       = 32
	MaxTracks      = 1536
	MaxKeys        = 100000
	MaxName        = 128
	MaxTextureSize = 2048
	MaxImageBytes  = MaxTextureSize*MaxTextureSize*5 + 22
	VertexStride   = 16
	Translation    = uint8(0)
	Rotation       = uint8(1)
	Scale          = uint8(2)
)

var ErrInvalid = errors.New("invalid bounded character resource")

// Document shares immutable clip banks between independently bound models.
type Document struct {
	Models []Model
	Banks  []Bank
}

type Node struct {
	Name        string
	Parent      int
	Translation [3]float32
	Rotation    [4]float32
	Scale       [3]float32
}

type Model struct {
	ID          string
	Bank        int
	Nodes       []Node
	MeshNode    int
	Joints      []uint16
	InverseBind []float32
	// Vertices pack position, normal, UV, four palette indices and four weights.
	Vertices      []float32
	Indices       []uint32
	Width, Height int
	AlbedoQOI     []byte
}

type Bank struct {
	ID    string
	Clips []Clip
}

type Clip struct {
	Name     string
	Duration float32
	Tracks   []Track
}

// Track targets canonical node names, independent of each model's node order.
// Step selects STEP interpolation; otherwise values use LINEAR interpolation.
type Track struct {
	Node          string
	Path          uint8
	Step          bool
	Times, Values []float32
}
