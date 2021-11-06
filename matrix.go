package jank3d

import (
	"math"
	"strconv"

	"github.com/kvartborg/vector"
)

type Matrix4 [4][4]float64

func NewMatrix4() Matrix4 {

	mat := Matrix4{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
		{0, 0, 1, 0},
		{0, 0, 0, 1},
	}
	return mat

}

func Translate(x, y, z float64) Matrix4 {
	mat := NewMatrix4()
	mat[3][0] = x
	mat[3][1] = y
	mat[3][2] = z
	return mat
}

func Scale(x, y, z float64) Matrix4 {
	mat := NewMatrix4()
	mat[0][0] = x
	mat[1][1] = y
	mat[2][2] = z
	return mat
}

func Rotate(x, y, z float64, angle float64) Matrix4 {

	mat := NewMatrix4()
	vector := vector.Vector{x, y, z}.Unit()
	s := math.Sin(angle)
	c := math.Cos(angle)
	m := 1 - c

	mat[0][0] = m*vector[0]*vector[0] + c
	mat[0][1] = m*vector[0]*vector[1] + vector[2]*s
	mat[0][2] = m*vector[2]*vector[0] - vector[1]*s

	mat[1][0] = m*vector[0]*vector[1] - vector[2]*s
	mat[1][1] = m*vector[1]*vector[1] + c
	mat[1][2] = m*vector[1]*vector[2] + vector[0]*s

	mat[2][0] = m*vector[2]*vector[0] + vector[1]*s
	mat[2][1] = m*vector[1]*vector[2] - vector[0]*s
	mat[2][2] = m*vector[2]*vector[2] + c

	return mat

}

func (matrix Matrix4) Right() vector.Vector {
	return vector.Vector{
		matrix[0][0],
		matrix[0][1],
		matrix[0][2],
	}.Unit()
}

func (matrix Matrix4) Up() vector.Vector {
	return vector.Vector{
		matrix[1][0],
		matrix[1][1],
		matrix[1][2],
	}.Unit()
}

func (matrix Matrix4) Forward() vector.Vector {
	return vector.Vector{
		matrix[2][0],
		matrix[2][1],
		matrix[2][2],
	}.Unit()
}

func Perspective(fovy, near, far, viewWidth, viewHeight float64) Matrix4 {

	aspect := viewWidth / viewHeight

	t := math.Tan(fovy * math.Pi / 360)
	b := -t
	r := t * aspect
	l := -r

	// l := -viewWidth / 2
	// r := viewWidth / 2
	// t := -viewHeight / 2
	// b := viewHeight / 2

	return Matrix4{
		{(2 * near) / (r - l), 0, (r + l) / (r - l), 0},
		{0, (2 * near) / (t - b), (t + b) / (t - b), 0},
		{0, 0, -((far + near) / (far - near)), -((2 * far * near) / (far - near))},
		{0, 0, -1, 0},
	}

}

// vvv WORKING, but not using fovy

// func Perspective(fovy, near, far, viewWidth, viewHeight float64) Matrix4 {

// 	l := -viewWidth / 2
// 	r := viewWidth / 2
// 	t := -viewHeight / 2
// 	b := viewHeight / 2

// 	return Matrix4{
// 		{2 * near / (r - l), 0, (r + l) / (r - l), 0},
// 		{0, (2 * near) / (t - b), (t + b) / (t - b), 0},
// 		{0, 0, (-far - near) / (far - near), (-2 * near) / (far - near)},
// 		{0, 0, -1, 0},
// 	}

// }

// func Perspective(fovy, near, far, viewWidth, viewHeight float64) Matrix4 {
// 	tan := math.Tan(fovy / 2)

// 	return Matrix4{
// 		{1.0 / (viewWidth / viewHeight * tan), 0, 0, 0},
// 		{0, 1.0 / tan, 0, 0},
// 		{0, 0, -((far + near) / (far - near)), -((2 * far * near) / (far - near))},
// 		{0, 0, -1, 0},
// 	}
// }

// Cribbed from https://github.com/fogleman/fauxgl vvvvvv

// func frustum(l, r, b, t, n, f float64) Matrix4 {
// 	t1 := 2 * n
// 	t2 := r - l
// 	t3 := t - b
// 	t4 := f - n
// 	return Matrix4{
// 		// {t1 / t2, 0, (r + l) / t2, 0},
// 		// {0, t1 / t3, (t + b) / t3, 0},
// 		// {0, 0, (-f - n) / t4, (-t1 * f) / t4},
// 		// {0, 0, -1, 0},

// 		{t1 / t2, 0, (r + l) / t2, 0},
// 		{0, t1 / t3, (t + b) / t3, 0},
// 		{0, 0, (-f - n) / t4, (-t1 * f) / t4},
// 		{0, 0, -1, 0},
// 	}
// }

// func (camera *Camera) SetOrthographicView() {

// 	w, h := camera.ColorTexture.Size()

// 	width := float64(w)
// 	height := float64(h)

// 	l, t := -width/2, -height/2
// 	r, b := width/2, height/2
// 	n, f := camera.Near, camera.Far

// 	camera.Projection = Matrix4{
// 		{2 / (r - l), 0, 0, -(r + l) / (r - l)},
// 		{0, 2 / (t - b), 0, -(t + b) / (t - b)},
// 		{0, 0, -2 / (f - n), -(f + n) / (f - n)},
// 		{0, 0, 0, 1}}
// }

//Cribbed from https://github.com/fogleman/fauxgl ^^^^^^^

func (matrix Matrix4) Rotate(v vector.Vector, angle float64) Matrix4 {
	mat := matrix.Clone()
	mat.MultVec(v)
	return mat
}

func (matrix Matrix4) Clone() Matrix4 {
	newMat := NewMatrix4()
	for y := 0; y < len(matrix); y++ {
		for x := 0; x < len(matrix[y]); x++ {
			newMat[y][x] = matrix[y][x]
		}
	}
	return newMat
}

func (matrix Matrix4) MultVec(vect vector.Vector) vector.Vector {

	return vector.Vector{

		matrix[0][0]*vect[0] + matrix[1][0]*vect[1] + matrix[2][0]*vect[2] + matrix[3][0],
		matrix[0][1]*vect[0] + matrix[1][1]*vect[1] + matrix[2][1]*vect[2] + matrix[3][1],
		matrix[0][2]*vect[0] + matrix[1][2]*vect[1] + matrix[2][2]*vect[2] + matrix[3][2],
		// matrix[3][0]*vect[0] + matrix[3][1]*vect[1] + matrix[3][2]*vect[2] + matrix[3][3],
		// 1 - vect[2] /

	}

}

func (matrix Matrix4) MultVecW(vect vector.Vector) vector.Vector {

	return vector.Vector{
		matrix[0][0]*vect[0] + matrix[1][0]*vect[1] + matrix[2][0]*vect[2] + matrix[3][0],
		matrix[0][1]*vect[0] + matrix[1][1]*vect[1] + matrix[2][1]*vect[2] + matrix[3][1],
		matrix[0][2]*vect[0] + matrix[1][2]*vect[1] + matrix[2][2]*vect[2] + matrix[3][2],
		matrix[0][3]*vect[0] + matrix[1][3]*vect[1] + matrix[2][3]*vect[2] + matrix[3][3],
	}

}

func (matrix Matrix4) Mult(other Matrix4) Matrix4 {

	newMat := NewMatrix4()

	newMat[0][0] = matrix[0][0]*other[0][0] + matrix[0][1]*other[1][0] + matrix[0][2]*other[2][0] + matrix[0][3]*other[3][0]
	newMat[1][0] = matrix[1][0]*other[0][0] + matrix[1][1]*other[1][0] + matrix[1][2]*other[2][0] + matrix[1][3]*other[3][0]
	newMat[2][0] = matrix[2][0]*other[0][0] + matrix[2][1]*other[1][0] + matrix[2][2]*other[2][0] + matrix[2][3]*other[3][0]
	newMat[3][0] = matrix[3][0]*other[0][0] + matrix[3][1]*other[1][0] + matrix[3][2]*other[2][0] + matrix[3][3]*other[3][0]

	newMat[0][1] = matrix[0][0]*other[0][1] + matrix[0][1]*other[1][1] + matrix[0][2]*other[2][1] + matrix[0][3]*other[3][1]
	newMat[1][1] = matrix[1][0]*other[0][1] + matrix[1][1]*other[1][1] + matrix[1][2]*other[2][1] + matrix[1][3]*other[3][1]
	newMat[2][1] = matrix[2][0]*other[0][1] + matrix[2][1]*other[1][1] + matrix[2][2]*other[2][1] + matrix[2][3]*other[3][1]
	newMat[3][1] = matrix[3][0]*other[0][1] + matrix[3][1]*other[1][1] + matrix[3][2]*other[2][1] + matrix[3][3]*other[3][1]

	newMat[0][2] = matrix[0][0]*other[0][2] + matrix[0][1]*other[1][2] + matrix[0][2]*other[2][2] + matrix[0][3]*other[3][2]
	newMat[1][2] = matrix[1][0]*other[0][2] + matrix[1][1]*other[1][2] + matrix[1][2]*other[2][2] + matrix[1][3]*other[3][2]
	newMat[2][2] = matrix[2][0]*other[0][2] + matrix[2][1]*other[1][2] + matrix[2][2]*other[2][2] + matrix[2][3]*other[3][2]
	newMat[3][2] = matrix[3][0]*other[0][2] + matrix[3][1]*other[1][2] + matrix[3][2]*other[2][2] + matrix[3][3]*other[3][2]

	newMat[0][3] = matrix[0][0]*other[0][3] + matrix[0][1]*other[1][3] + matrix[0][2]*other[2][3] + matrix[0][3]*other[3][3]
	newMat[1][3] = matrix[1][0]*other[0][3] + matrix[1][1]*other[1][3] + matrix[1][2]*other[2][3] + matrix[1][3]*other[3][3]
	newMat[2][3] = matrix[2][0]*other[0][3] + matrix[2][1]*other[1][3] + matrix[2][2]*other[2][3] + matrix[2][3]*other[3][3]
	newMat[3][3] = matrix[3][0]*other[0][3] + matrix[3][1]*other[1][3] + matrix[3][2]*other[2][3] + matrix[3][3]*other[3][3]

	return newMat

}

func (matrix Matrix4) Columns() [][]float64 {

	columns := [][]float64{
		{0, 0, 0, 0},
		{0, 0, 0, 0},
		{0, 0, 0, 0},
		{0, 0, 0, 0},
	}

	for r := range matrix {
		for c := range matrix[r] {
			columns[c][r] = matrix[r][c]
		}
	}

	return columns
}

// func (matrix Matrix4) RowSums() []float64 {
// 	rowSums := []float64{}
// 	for _, row := range matrix {
// 		s := 0.0
// 		for _, c := range row {
// 			s += c
// 		}
// 		rowSums = append(rowSums, s)
// 	}
// 	return rowSums
// }

// func (matrix Matrix4) ColumnSums() []float64 {

// 	columnSums := []float64{}
// 	for x := 0; x < 4; x++ {
// 		s := 0.0
// 		for rowIndex := range matrix {
// 			s += matrix[x][rowIndex]
// 		}
// 		columnSums = append(columnSums, s)
// 	}
// 	return columnSums

// }

func (matrix Matrix4) String() string {
	s := "{"
	for i, y := range matrix {
		for _, x := range y {
			s += strconv.FormatFloat(x, 'f', -1, 64) + ", "
		}
		if i < len(matrix)-1 {
			s += "\n"
		}
	}
	s += "}"
	return s
}

func LookAt(eye, center, up vector.Vector) Matrix4 {
	z := eye.Sub(center).Unit()
	x, _ := up.Cross(z)
	x = x.Unit()
	y, _ := z.Cross(x)
	return Matrix4{
		{x[0], x[1], x[2], -x.Dot(eye)},
		{y[0], y[1], y[2], -y.Dot(eye)},
		{z[0], z[1], z[2], -z.Dot(eye)},
		{0, 0, 0, 1},
	}
}

// func LookAt(target, center, up vector.Vector) Matrix4 {
// 	z := target.Sub(center).Unit()
// 	x, _ := up.Cross(z)
// 	x = x.Unit()
// 	y, _ := z.Cross(x)

// 	return Matrix4{
// 		{x[0], x[1], x[2], -x.Dot(center)},
// 		{y[0], y[1], y[2], -y.Dot(center)},
// 		{z[0], z[1], z[2], -z.Dot(center)},
// 		{0, 0, 0, 1},
// 	}

// }
