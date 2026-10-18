package worldterrain

import "math"

// CompilePatch generates one immutable, indexed upload at mount time. Both
// triangles use the fixed lower-left/upper-right diagonal; holes emit none.
func CompilePatch(d *Document, index int) (Mesh, error) {
	if err := Validate(d); err != nil {
		return Mesh{}, err
	}
	if index < 0 || index >= len(d.Patches) {
		return Mesh{}, ErrBounds
	}
	return compilePatch(d, index), nil
}

// CompilePatches validates the complete document once and compiles every patch.
// It preserves CompilePatch's geometry, ordering and owned mesh storage.
func CompilePatches(d *Document) ([]Mesh, error) {
	if err := Validate(d); err != nil {
		return nil, err
	}
	meshes := make([]Mesh, len(d.Patches))
	for index := range meshes {
		meshes[index] = compilePatch(d, index)
	}
	return meshes, nil
}

func compilePatch(d *Document, index int) Mesh {
	p := d.Patches[index]
	cols, rows := int(p.Columns)+1, int(p.Rows)+1
	mesh := Mesh{
		Vertices: make([]Vertex, cols*rows),
		Indices:  make([]uint32, 0, int(p.Columns)*int(p.Rows)*6),
		Bounds:   [6]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32},
	}
	for y := range rows {
		for x := range cols {
			gx, gy := int(p.X)+x, int(p.Y)+y
			global := gy*int(d.Columns) + gx
			pos := [3]float32{d.Origin[0] + float32(gx)*d.CellSize, d.Origin[1] + float32(gy)*d.CellSize, d.Heights[global]}
			mesh.Vertices[y*cols+x] = Vertex{Position: pos, Normal: d.Normals[global]}
			for a, v := range pos {
				mesh.Bounds[a] = min(mesh.Bounds[a], v)
				mesh.Bounds[a+3] = max(mesh.Bounds[a+3], v)
			}
		}
	}
	for y := range int(p.Rows) {
		for x := range int(p.Columns) {
			if d.Holes[(int(p.Y)+y)*(int(d.Columns)-1)+int(p.X)+x] != 0 {
				continue
			}
			a := uint32(y*cols + x)
			b, c := a+1, a+uint32(cols)
			mesh.Indices = append(mesh.Indices, a, b, c, b, c+1, c)
		}
	}
	return mesh
}

// HeightAt samples the same triangle planes used by rendering and collision.
// The caller supplies an already validated immutable document.
func HeightAt(d *Document, x, y float64) (float64, bool) {
	if d == nil || d.Columns < 2 || d.Rows < 2 || d.CellSize <= 0 || math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) ||
		math.IsInf(y, 0) {
		return 0, false
	}
	cx, fx, insideX := axisCell(d.Origin[0], d.CellSize, int(d.Columns), x)
	cy, fy, insideY := axisCell(d.Origin[1], d.CellSize, int(d.Rows), y)
	if !insideX || !insideY {
		return 0, false
	}
	cell := cy*(int(d.Columns)-1) + cx
	if cell >= len(d.Holes) || d.Holes[cell] != 0 {
		return 0, false
	}
	a := cy*int(d.Columns) + cx
	b, c := a+1, a+int(d.Columns)
	if c+1 >= len(d.Heights) {
		return 0, false
	}
	if fx+fy <= 1 {
		return float64(d.Heights[a])*(1-fx-fy) + float64(d.Heights[b])*fx + float64(d.Heights[c])*fy, true
	}
	return float64(d.Heights[b])*(1-fy) + float64(d.Heights[c+1])*(fx+fy-1) + float64(d.Heights[c])*(1-fx), true
}

// Use the rounded float32 positions shared by GPU and collision meshes. Their
// spacing can vary slightly even when every adjacent position is distinct.
func axisCell(origin, step float32, count int, value float64) (int, float64, bool) {
	coordinate := func(index int) float64 { return float64(origin + float32(index)*step) }
	if value < coordinate(0) || value > coordinate(count-1) {
		return 0, 0, false
	}
	low, high := 0, count-1
	for high-low > 1 {
		middle := (low + high) / 2
		if coordinate(middle) <= value {
			low = middle
		} else {
			high = middle
		}
	}
	width := coordinate(high) - coordinate(low)
	if width <= 0 {
		return 0, 0, false
	}
	return low, (value - coordinate(low)) / width, true
}
