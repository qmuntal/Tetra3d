package tetra3d

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"github.com/kvartborg/vector"
	"golang.org/x/image/font/basicfont"
)

// DebugInfo is a struct that holds debugging information for a Camera's render pass. These values are reset when Camera.Clear() is called.
type DebugInfo struct {
	AvgFrameTime     time.Duration // Amount of CPU frame time spent transforming vertices. Doesn't necessarily include CPU time spent sending data to the GPU.
	AvgAnimationTime time.Duration // Amount of CPU frame time spent animating vertices.
	AvgLightTime     time.Duration // Amount of CPU frame time spent lighting vertices.
	animationTime    time.Duration
	lightTime        time.Duration
	frameTime        time.Duration
	tickTime         time.Time
	DrawnParts       int // Number of draw calls, excluding those invisible or culled based on distance
	TotalParts       int // Total number of objects
	DrawnTris        int // Number of drawn triangles, excluding those hidden from backface culling
	TotalTris        int // Total number of triangles
	LightCount       int // Total number of lights
	ActiveLightCount int // Total active number of lights
}

// Camera represents a camera (where you look from) in Tetra3D.
type Camera struct {
	*Node
	ColorTexture          *ebiten.Image // ColorTexture holds the color results of rendering any models.
	DepthTexture          *ebiten.Image // DepthTexture holds the depth results of rendering any models, if Camera.RenderDepth is on.
	ColorIntermediate     *ebiten.Image
	DepthIntermediate     *ebiten.Image
	ClipAlphaIntermediate *ebiten.Image

	RenderDepth bool // If the Camera should attempt to render a depth texture; if this is true, then DepthTexture will hold the depth texture render results.

	DepthShader              *ebiten.Shader
	ClipAlphaCompositeShader *ebiten.Shader
	ClipAlphaRenderShader    *ebiten.Shader
	ColorShader              *ebiten.Shader
	Near, Far                float64 // The near and far clipping plane.
	Perspective              bool    // If the Camera has a perspective projection. If not, it would be orthographic
	FieldOfView              float64 // Vertical field of view in degrees for a perspective projection camera
	OrthoScale               float64 // Scale of the view for an orthographic projection camera in units horizontally

	DebugInfo DebugInfo

	FrustumSphere *BoundingSphere
	backfacePool  *VectorPool
}

// NewCamera creates a new Camera with the specified width and height.
func NewCamera(w, h int) *Camera {

	cam := &Camera{
		Node:        NewNode("Camera"),
		RenderDepth: true,
		Near:        0.1,
		Far:         100,

		FrustumSphere: NewBoundingSphere("camera frustum sphere", 0),
		backfacePool:  NewVectorPool(3),
	}

	depthShaderText := []byte(
		`package main

		func encodeDepth(depth float) vec4 {
			r := floor(depth * 255) / 255
			g := floor(fract(depth * 255) * 255) / 255
			b := fract(depth * 255*255)
			return vec4(r, g, b, 1);
		}

		func decodeDepth(rgba vec4) float {
			return rgba.r + (rgba.g / 255) + (rgba.b / 65025)
		}

		func Fragment(position vec4, texCoord vec2, color vec4) vec4 {

			existingDepth := imageSrc0At(position.xy / imageSrcTextureSize())

			if existingDepth.a == 0 || decodeDepth(existingDepth) > color.r {
				return encodeDepth(color.r)
			}

			return vec4(0.0, 0.0, 0.0, 0.0)

		}

		`,
	)

	var err error

	cam.DepthShader, err = ebiten.NewShader(depthShaderText)

	if err != nil {
		panic(err)
	}

	clipRenderText := []byte(
		`package main

		func encodeDepth(depth float) vec4 {
			r := floor(depth * 255) / 255
			g := floor(fract(depth * 255) * 255) / 255
			b := fract(depth * 255*255)
			return vec4(r, g, b, 1);
		}

		func Fragment(position vec4, texCoord vec2, color vec4) vec4 {
			tex := imageSrc0At(texCoord)
			return vec4(encodeDepth(color.r).rgb, tex.a)
			// TODO: This shader needs to discard if tex.a is transparent. We can't sample the texture to return 
			// what's underneath here, so discard is basically necessary. We need to implement it once the dicard
			// keyword / function is implemented (if it ever is; hopefully it will be).
		}

		`,
	)

	cam.ClipAlphaRenderShader, err = ebiten.NewShader(clipRenderText)

	if err != nil {
		panic(err)
	}

	clipCompositeShaderText := []byte(
		`package main

		func decodeDepth(rgba vec4) float {
			return rgba.r + (rgba.g / 255) + (rgba.b / 65025)
		}

		func Fragment(position vec4, texCoord vec2, color vec4) vec4 {

			depthValue := imageSrc0At(texCoord)
			texture := imageSrc1At(texCoord)

			if depthValue.a == 0 || decodeDepth(depthValue) > texture.r {
				return texture
			}

			return vec4(0.0, 0.0, 0.0, 0.0)

		}

		`,
	)

	cam.ClipAlphaCompositeShader, err = ebiten.NewShader(clipCompositeShaderText)

	if err != nil {
		panic(err)
	}

	colorShaderText := []byte(
		`package main

		var Fog vec4
		var FogRange [2]float

		func decodeDepth(rgba vec4) float {
			return rgba.r + (rgba.g / 255) + (rgba.b / 65025)
		}
		
		func Fragment(position vec4, texCoord vec2, color vec4) vec4 {

			depth := imageSrc1At(texCoord)
			
			if depth.a > 0 {
				colorTex := imageSrc0At(texCoord)
				
				d := smoothstep(FogRange[0], FogRange[1], decodeDepth(depth))

				if Fog.a == 1 {
					colorTex.rgb += Fog.rgb * d * colorTex.a
				} else if Fog.a == 2 {
					colorTex.rgb -= Fog.rgb * d * colorTex.a
				} else if Fog.a == 3 {
					colorTex.rgb = mix(colorTex.rgb, Fog.rgb, d) * colorTex.a
				}

				return colorTex
			}

			return vec4(0.0, 0.0, 0.0, 0.0)

		}

		`,
	)

	cam.ColorShader, err = ebiten.NewShader(colorShaderText)

	if err != nil {
		panic(err)
	}

	if w != 0 && h != 0 {
		cam.Resize(w, h)
	}

	cam.SetPerspective(60)

	return cam
}

func (camera *Camera) Clone() INode {

	w, h := camera.ColorTexture.Size()
	clone := NewCamera(w, h)

	clone.RenderDepth = camera.RenderDepth
	clone.Near = camera.Near
	clone.Far = camera.Far
	clone.Perspective = camera.Perspective
	clone.FieldOfView = camera.FieldOfView

	clone.Node = camera.Node.Clone().(*Node)
	for _, child := range camera.children {
		child.setParent(camera)
	}

	return clone

}

func (camera *Camera) Resize(w, h int) {

	if camera.ColorTexture != nil {
		camera.ColorTexture.Dispose()
		camera.DepthTexture.Dispose()
		camera.ColorIntermediate.Dispose()
		camera.DepthIntermediate.Dispose()
		camera.ClipAlphaIntermediate.Dispose()
	}

	camera.ColorTexture = ebiten.NewImage(w, h)
	camera.DepthTexture = ebiten.NewImage(w, h)
	camera.ColorIntermediate = ebiten.NewImage(w, h)
	camera.DepthIntermediate = ebiten.NewImage(w, h)
	camera.ClipAlphaIntermediate = ebiten.NewImage(w, h)

}

// ViewMatrix returns the Camera's view matrix.
func (camera *Camera) ViewMatrix() Matrix4 {

	transform := NewMatrix4Translate(-camera.WorldPosition()[0], -camera.WorldPosition()[1], -camera.WorldPosition()[2])

	// We invert the rotation because the Camera is looking down -Z
	transform = transform.Mult(camera.WorldRotation().Transposed())

	return transform

}

// Projection returns the Camera's projection matrix.
func (camera *Camera) Projection() Matrix4 {
	if camera.Perspective {
		return NewProjectionPerspective(camera.FieldOfView, camera.Near, camera.Far, float64(camera.ColorTexture.Bounds().Dx()), float64(camera.ColorTexture.Bounds().Dy()))
	}
	w, h := camera.ColorTexture.Size()
	asr := float64(h) / float64(w)

	return NewProjectionOrthographic(camera.Near, camera.Far, 1*camera.OrthoScale, -1*camera.OrthoScale, asr*camera.OrthoScale, -asr*camera.OrthoScale)
	// return NewProjectionOrthographic(camera.Near, camera.Far, float64(camera.ColorTexture.Bounds().Dx())*camera.OrthoScale, float64(camera.ColorTexture.Bounds().Dy())*camera.OrthoScale)
}

// SetPerspective sets the Camera's projection to be a perspective projection. fovY indicates the vertical field of view (in degrees) for the camera's aperture.
func (camera *Camera) SetPerspective(fovY float64) {
	camera.FieldOfView = fovY
	camera.Perspective = true
}

// SetOrthographic sets the Camera's projection to be an orthographic projection. orthoScale indicates the scale of the camera in units horizontally.
func (camera *Camera) SetOrthographic(orthoScale float64) {
	camera.Perspective = false
	camera.OrthoScale = orthoScale
}

// We do this for each vertex for each triangle for each model, so we want to avoid allocating vectors if possible. clipToScreen
// does this by taking outVec, a vertex (vector.Vector) that it stores the values in and returns, which avoids reallocation.
func (camera *Camera) clipToScreen(vert, outVec vector.Vector, mat *Material, width, height float64) vector.Vector {

	v3 := vert[3]

	if !camera.Perspective {
		v3 = 1.0
	}

	// If the trangle is beyond the screen, we'll just pretend it's not and limit it to the closest possible value > 0
	// TODO: Replace this with triangle clipping or fix whatever graphical glitch seems to arise periodically
	if v3 < 0 {
		v3 = 0.000001
	}

	// Again, this function should only be called with pre-transformed 4D vertex arguments.

	outVec[0] = (vert[0]/v3)*width + (width / 2)
	outVec[1] = (vert[1]/v3*-1)*height + (height / 2)
	outVec[2] = vert[2]
	outVec[3] = vert[3]

	if mat != nil && mat.ClipProgram != nil {
		outVec = mat.ClipProgram(outVec)
	}

	return outVec

}

// ClipToScreen projects the pre-transformed vertex in View space and remaps it to screen coordinates.
func (camera *Camera) ClipToScreen(vert vector.Vector) vector.Vector {
	width, height := camera.ColorTexture.Size()
	return camera.clipToScreen(vert, vector.Vector{0, 0, 0, 0}, nil, float64(width), float64(height))
}

// WorldToScreen transforms a 3D position in the world to screen coordinates.
func (camera *Camera) WorldToScreen(vert vector.Vector) vector.Vector {
	v := NewMatrix4Translate(vert[0], vert[1], vert[2]).Mult(camera.ViewMatrix().Mult(camera.Projection()))
	return camera.ClipToScreen(v.MultVecW(vector.Vector{0, 0, 0}))
}

// WorldToClip transforms a 3D position in the world to clip coordinates (before screen normalization).
func (camera *Camera) WorldToClip(vert vector.Vector) vector.Vector {
	v := NewMatrix4Translate(vert[0], vert[1], vert[2]).Mult(camera.ViewMatrix().Mult(camera.Projection()))
	return v.MultVecW(vector.Vector{0, 0, 0})
}

// Clear should be called at the beginning of a single rendered frame. It clears the backing textures before rendering.
func (camera *Camera) Clear() {

	camera.ColorTexture.Clear()
	camera.DepthTexture.Clear()

	if camera.DebugInfo.tickTime.IsZero() || time.Since(camera.DebugInfo.tickTime).Milliseconds() >= 100 {
		camera.DebugInfo.tickTime = time.Now()
		camera.DebugInfo.AvgFrameTime = camera.DebugInfo.frameTime
		camera.DebugInfo.AvgAnimationTime = camera.DebugInfo.animationTime
		camera.DebugInfo.AvgLightTime = camera.DebugInfo.lightTime
	}
	camera.DebugInfo.frameTime = 0
	camera.DebugInfo.animationTime = 0
	camera.DebugInfo.lightTime = 0
	camera.DebugInfo.DrawnParts = 0
	camera.DebugInfo.TotalParts = 0
	camera.DebugInfo.TotalTris = 0
	camera.DebugInfo.DrawnTris = 0
	camera.DebugInfo.LightCount = 0
	camera.DebugInfo.ActiveLightCount = 0
}

// RenderNodes renders all nodes starting with the provided rootNode using the Scene's properties (fog, for example). Note that if Camera.RenderDepth
// is false, scenes rendered one after another in multiple RenderNodes() calls will be rendered on top of each other in the Camera's texture buffers.
// Note that for Models, each MeshPart of a Model has a maximum renderable triangle count of 21845.
func (camera *Camera) RenderNodes(scene *Scene, rootNode INode) {

	meshes := []*Model{}

	if model, isModel := rootNode.(*Model); isModel {
		meshes = append(meshes, model)
	}

	nodes := rootNode.ChildrenRecursive()

	for _, node := range nodes {
		if model, ok := node.(*Model); ok {
			meshes = append(meshes, model)
		}
	}

	camera.Render(scene, meshes...)

}

// func (camera *Camera) RenderNodes(scene *Scene, nodes ...Node) {

// 	meshes := []*Model{}

// 	for _, node := range nodes {

// 		if model, isModel := node.(*Model); isModel {
// 			meshes = append(meshes, model)
// 		}

// 	}

// 	camera.Render(scene, meshes...)

// }

type RenderPair struct {
	Model    *Model
	MeshPart *MeshPart
}

// Render renders all of the models passed using the provided Scene's properties (fog, for example). Note that if Camera.RenderDepth
// is false, scenes rendered one after another in multiple Render() calls will be rendered on top of each other in the Camera's texture buffers.
// Note that for Models, each MeshPart of a Model has a maximum renderable triangle count of 21845.
func (camera *Camera) Render(scene *Scene, models ...*Model) {

	frametimeStart := time.Now()

	lights := []Light{}

	if scene.LightingOn {

		for _, l := range scene.Root.ChildrenRecursive() {
			if light, isLight := l.(Light); isLight {
				camera.DebugInfo.LightCount++
				if light.isOn() {
					lights = append(lights, light)
					light.beginRender()
					camera.DebugInfo.ActiveLightCount++
				}
			}
		}

	}

	// By multiplying the camera's position against the view matrix (which contains the negated camera position), we're left with just the rotation
	// matrix, which we feed into model.TransformedVertices() to draw vertices in order of distance.
	vpMatrix := camera.ViewMatrix().Mult(camera.Projection())

	// Update the camera's frustum sphere
	dist := (camera.Far - camera.Near) / 2
	forward := camera.WorldRotation().Forward().Invert()

	camera.FrustumSphere.SetWorldPosition(camera.WorldPosition().Add(forward.Scale(camera.Near + dist)))
	camera.FrustumSphere.Radius = dist * 1.5

	rectShaderOptions := &ebiten.DrawRectShaderOptions{}
	rectShaderOptions.Images[0] = camera.ColorIntermediate
	rectShaderOptions.Images[1] = camera.DepthIntermediate

	if scene != nil {

		rectShaderOptions.Uniforms = map[string]interface{}{
			"Fog":      scene.fogAsFloatSlice(),
			"FogRange": scene.FogRange,
		}

	} else {

		rectShaderOptions.Uniforms = map[string]interface{}{
			"Fog":      []float32{0, 0, 0, 0},
			"FogRange": []float32{0, 1},
		}

	}

	// Reusing vectors rather than reallocating for all triangles for all models
	p0 := vector.Vector{0, 0, 0, 0}
	p1 := vector.Vector{0, 0, 0, 0}
	p2 := vector.Vector{0, 0, 0, 0}

	solids := []RenderPair{}
	transparents := []RenderPair{}

	for _, model := range models {
		if model.Mesh != nil {
			for _, mp := range model.Mesh.MeshParts {
				if model.isTransparent(mp) {
					transparents = append(transparents, RenderPair{model, mp})
				} else {
					solids = append(solids, RenderPair{model, mp})
				}
			}
		}
	}

	// If the camera isn't rendering depth, then we should sort models by distance to ensure things draw in something like the correct order
	if len(solids) > 0 && !camera.RenderDepth {

		sort.SliceStable(solids, func(i, j int) bool {
			return fastVectorDistanceSquared(solids[i].Model.WorldPosition(), camera.WorldPosition()) > fastVectorDistanceSquared(solids[j].Model.WorldPosition(), camera.WorldPosition())
		})

	}

	camWidth, camHeight := camera.ColorTexture.Size()

	render := func(renderPair RenderPair) {

		model := renderPair.Model
		meshPart := renderPair.MeshPart

		// Models without Meshes are essentially just "nodes" that just have a position. They aren't counted for rendering.
		if model.Mesh == nil {
			return
		}

		camera.DebugInfo.TotalParts++
		camera.DebugInfo.TotalTris += len(meshPart.Triangles)

		if !model.visible {
			return
		}

		if model.FrustumCulling {

			// We simply call this to update the bounding sphere as necessary. Skinned meshes don't actually call this for Model.TransformedVertices(), and rather than putting it there,
			// it seems better to put it here to ensure the sphere is in the right position since it's possible FrustumCulling is on but the model has never been rendered, it never updates
			// BoundingSphere, and so remains invisible.
			model.Transform()

			if !model.BoundingSphere.Intersecting(camera.FrustumSphere) {
				return
			}

		}

		camera.DebugInfo.DrawnParts++

		tris := model.TransformedVertices(vpMatrix, camera, meshPart)

		vertexListIndex := 0

		backfaceCulling := true
		if meshPart.Material != nil {
			backfaceCulling = meshPart.Material.BackfaceCulling
		}

		srcW := 0.0
		srcH := 0.0

		if meshPart.Material != nil && meshPart.Material.Texture != nil {
			srcW = float64(meshPart.Material.Texture.Bounds().Dx())
			srcH = float64(meshPart.Material.Texture.Bounds().Dy())
		}

		var img *ebiten.Image

		if meshPart.Material != nil {
			img = meshPart.Material.Texture
		}

		if img == nil {
			img = defaultImg
		}

		for _, tri := range tris {

			if !tri.visible {
				continue
			}

			v0 := tri.Vertices[0].transformed
			v1 := tri.Vertices[1].transformed
			v2 := tri.Vertices[2].transformed

			// Near-ish clipping (basically clip triangles that are wholly behind the camera)
			if v0[3] < 0 && v1[3] < 0 && v2[3] < 0 {
				continue
			}

			if v0[2] > camera.Far && v1[2] > camera.Far && v2[2] > camera.Far {
				continue
			}

			// Backface Culling

			// if model.BackfaceCulling {

			// 	// SHOUTOUTS TO MOD DB FOR POINTING ME IN THE RIGHT DIRECTION FOR THIS BECAUSE GOOD LORDT:
			// 	// https://moddb.fandom.com/wiki/Backface_culling#Polygons_in_object_space_are_transformed_into_world_space

			// 	// We use Vertex.transformed[:3] here because the fourth W component messes up normal calculation otherwise
			// 	normal := calculateNormal(tri.Vertices[0].transformed[:3], tri.Vertices[1].transformed[:3], tri.Vertices[2].transformed[:3])

			// 	dot := normal.Dot(tri.Vertices[0].transformed[:3])

			// 	// A little extra to make sure we draw walls if you're peeking around them with a higher FOV
			// 	if dot < -0.1 {
			// 		continue
			// 	}

			// }

			p0 = camera.clipToScreen(v0, p0, meshPart.Material, float64(camWidth), float64(camHeight))
			p1 = camera.clipToScreen(v1, p1, meshPart.Material, float64(camWidth), float64(camHeight))
			p2 = camera.clipToScreen(v2, p2, meshPart.Material, float64(camWidth), float64(camHeight))

			// We can skip triangles that lie entirely outside of the view horizontally and vertically.
			if (p0[0] < 0 && p1[0] < 0 && p2[0] < 0) ||
				(p0[1] < 0 && p1[1] < 0 && p2[1] < 0) ||
				(p0[0] > float64(camWidth) && p1[0] > float64(camWidth) && p2[0] > float64(camWidth)) ||
				(p0[1] > float64(camHeight) && p1[1] > float64(camHeight) && p2[1] > float64(camHeight)) {
				continue
			}

			// This is a bit of a hacky way to do backface culling; it works, but it uses
			// the screen positions of the vertices to determine if the triangle should be culled.
			// In truth, it would be better to use the above approach, but that gives us visual
			// errors when faces are behind the camera unless we clip triangles. I don't really
			// feel like doing that right now, so here we are.

			if backfaceCulling {

				camera.backfacePool.Reset()
				n0 := camera.backfacePool.Sub(p0, p1)[:3]
				n1 := camera.backfacePool.Sub(p1, p2)[:3]
				nor := camera.backfacePool.Cross(n0, n1)

				if nor[2] > 0 {
					continue
				}

			}

			t := vertexListIndex / 3

			// Enforce maximum vertex count; note that this is lazy, which is NOT really a good way of doing this, as you can't really know ahead of time how many triangles may render.
			if t >= ebiten.MaxIndicesNum/3 {
				panic("error in rendering mesh [" + model.Mesh.Name + "] of model [" + model.name + "]. At " + fmt.Sprintf("%d", model.Mesh.TotalTriangleCount()) + " triangles, it exceeds the maximum of 21845 rendered triangles total for one MeshPart; please break up the mesh into multiple MeshParts using materials, or split it up into models")
			}

			triList[t] = tri

			vertexList[vertexListIndex].DstX = float32(math.Round(p0[0]))
			vertexList[vertexListIndex].DstY = float32(math.Round(p0[1]))

			vertexList[vertexListIndex+1].DstX = float32(math.Round(p1[0]))
			vertexList[vertexListIndex+1].DstY = float32(math.Round(p1[1]))

			vertexList[vertexListIndex+2].DstX = float32(math.Round(p2[0]))
			vertexList[vertexListIndex+2].DstY = float32(math.Round(p2[1]))

			if camera.RenderDepth {

				far := camera.Far
				if !camera.Perspective {
					far = 2.0

				}

				for i, vert := range tri.Vertices {

					depth := vert.transformed[2] / far
					if depth < 0 {
						depth = 0
					}

					vertexList[vertexListIndex+i].ColorR = float32(depth)
					vertexList[vertexListIndex+i].ColorG = float32(depth)
					vertexList[vertexListIndex+i].ColorB = float32(depth)
					vertexList[vertexListIndex+i].ColorA = 1

					// We set the UVs back here because we might need to use them if the material has clip alpha enabled.
					vertexList[vertexListIndex+i].SrcX = float32(vert.UV[0] * srcW)

					// We do 1 - v here (aka Y in texture coordinates) because 1.0 is the top of the texture while 0 is the bottom in UV coordinates,
					// but when drawing textures 0 is the top, and the sourceHeight is the bottom.
					vertexList[vertexListIndex+i].SrcY = float32((1 - vert.UV[1]) * srcH)

				}

			}

			vertexListIndex += 3

		}

		if vertexListIndex == 0 {
			return
		}

		for i := 0; i < vertexListIndex; i++ {
			indexList[i] = uint16(i)
		}

		// Render the depth map here
		if camera.RenderDepth {

			// OK, so the general process for rendering to the depth texture is three-fold:
			// 1) For solid objects, we simply render all triangles using camera.DepthShader. This draws triangles using their vertices'
			// color channels to indicate depth. It reads camera.DepthTexture to discard fragments previously rendered with a darker color
			// (and so are closer, as the color ranges from 0 (black, close) to 1 (white, far)).

			// 2) For transparent objects, we do the above, but don't write the transparent object to the DepthTexture.
			// By not rendering the object to the DepthTexture but still rendering it to the DepthIntermediate texture, the object still renders to
			// camera.ColorTexture visibly, but does not obscure any objects behind them by writing to the depth texture, thereby
			// allowing them to be seen through the transparent materials.
			// Transparent objects are rendered in a second pass, in far-to-close ordering.

			// 3) For alpha clip objects, we first render the triangles to an intermediate texture (camera.AlphaClipIntermediate). We use the camera.ClipAlphaRenderShader to
			// render the triangles with their vertex colors, and use the texture's alpha channel for clipping. We then draw the intermediate render
			// to DepthIntermediate using the camera.ClipAlphaShaderComposite shader, which is used to both discard previous closer fragments on camera.DepthTexture,
			// as well as cut out the alpha of the actual texture according to the ClipIntermediate image. This ClipIntermediate rendering step would be unnecessary if we could render to DepthIntermediate using the material's
			// image while also reading the DepthTexture, but unfortunately, images can't currently be different sizes in Ebiten.
			// See: https://github.com/hajimehoshi/ebiten/issues/1870

			transparencyMode := TransparencyModeOpaque

			if meshPart.Material != nil {
				transparencyMode = meshPart.Material.TransparencyMode
			}

			camera.DepthIntermediate.Clear()

			if transparencyMode == TransparencyModeAlphaClip {

				camera.ClipAlphaIntermediate.Clear()

				camera.ClipAlphaIntermediate.DrawTrianglesShader(vertexList[:vertexListIndex], indexList[:vertexListIndex], camera.ClipAlphaRenderShader, &ebiten.DrawTrianglesShaderOptions{Images: [4]*ebiten.Image{img}})

				w, h := camera.DepthIntermediate.Size()

				camera.DepthIntermediate.DrawRectShader(w, h, camera.ClipAlphaCompositeShader, &ebiten.DrawRectShaderOptions{Images: [4]*ebiten.Image{camera.DepthTexture, camera.ClipAlphaIntermediate}})

			} else {
				shaderOpt := &ebiten.DrawTrianglesShaderOptions{
					Images: [4]*ebiten.Image{camera.DepthTexture},
				}

				camera.DepthIntermediate.DrawTrianglesShader(vertexList[:vertexListIndex], indexList[:vertexListIndex], camera.DepthShader, shaderOpt)
			}

			if !model.isTransparent(meshPart) {
				camera.DepthTexture.DrawImage(camera.DepthIntermediate, nil)
			}

		}

		index := 0

		for _, tri := range triList[:vertexListIndex/3] {

			for _, vert := range tri.Vertices {

				// Vertex colors

				vertexList[index].ColorR = vert.Color.R
				vertexList[index].ColorG = vert.Color.G
				vertexList[index].ColorB = vert.Color.B

				vertexList[index].ColorA = vert.Color.A

				index++

			}

		}

		lighting := scene.LightingOn
		if meshPart.Material != nil {
			lighting = scene.LightingOn && !meshPart.Material.Shadeless
		}

		if lighting {

			index = 0

			t := time.Now()

			for _, light := range lights {
				light.beginModel(model, camera)
			}

			lightColors := [9]float32{}

			for _, tri := range triList[:vertexListIndex/3] {

				for i := range lightColors {
					lightColors[i] = 0
				}

				for _, light := range lights {
					for i, v := range light.Light(tri) {
						lightColors[i] += v
					}
				}

				for vertIndex := range tri.Vertices {

					vertexList[index].ColorR *= lightColors[(vertIndex)*3]
					vertexList[index].ColorG *= lightColors[(vertIndex)*3+1]
					vertexList[index].ColorB *= lightColors[(vertIndex)*3+2]
					index++

				}

			}

			camera.DebugInfo.lightTime += time.Since(t)

		}

		t := &ebiten.DrawTrianglesOptions{}
		t.ColorM = model.ColorBlendingFunc(model, meshPart) // Modify the model's appearance using its color blending function
		if meshPart.Material != nil {
			t.Filter = meshPart.Material.TextureFilterMode
			t.Address = meshPart.Material.TextureWrapMode
		}

		hasFragShader := meshPart.Material != nil && meshPart.Material.fragmentShader != nil && meshPart.Material.FragmentShaderOn
		w, h := camera.ColorTexture.Size()

		// If rendering depth, and rendering through a custom fragment shader, we'll need to render the tris to the ColorIntermediate buffer using the custom shader.
		// If we're not rendering through a custom shader, we can render to ColorIntermediate and then composite that onto the finished ColorTexture.

		// If we're not rendering depth, but still rendering through the shader, we can render to the intermediate texture, and then from there composite.
		// Otherwise

		if camera.RenderDepth {

			camera.ColorIntermediate.Clear()

			if hasFragShader {
				camera.ColorIntermediate.DrawTrianglesShader(vertexList[:vertexListIndex], indexList[:vertexListIndex], meshPart.Material.fragmentShader, meshPart.Material.FragmentShaderOptions)
			} else {
				camera.ColorIntermediate.DrawTriangles(vertexList[:vertexListIndex], indexList[:vertexListIndex], img, t)
			}

			camera.ColorTexture.DrawRectShader(w, h, camera.ColorShader, rectShaderOptions)

		} else {

			if hasFragShader {
				camera.ColorTexture.DrawTrianglesShader(vertexList[:vertexListIndex], indexList[:vertexListIndex], meshPart.Material.fragmentShader, meshPart.Material.FragmentShaderOptions)
			} else {
				camera.ColorTexture.DrawTriangles(vertexList[:vertexListIndex], indexList[:vertexListIndex], img, t)
			}

		}

		camera.DebugInfo.DrawnTris += vertexListIndex / 3

	}

	for _, renderPair := range solids {
		render(renderPair)
	}

	if len(transparents) > 0 {

		sort.SliceStable(transparents, func(i, j int) bool {
			return fastVectorDistanceSquared(transparents[i].Model.WorldPosition(), camera.WorldPosition()) > fastVectorDistanceSquared(transparents[j].Model.WorldPosition(), camera.WorldPosition())
		})

		for _, renderPair := range transparents {
			render(renderPair)
		}

	}

	camera.DebugInfo.frameTime += time.Since(frametimeStart)

}

func (camera *Camera) drawCircle(screen *ebiten.Image, position vector.Vector, radius float64, drawColor color.Color) {

	transformedCenter := camera.WorldToScreen(position)

	stepCount := float64(32)

	for i := 0; i < int(stepCount); i++ {

		x := (math.Sin(math.Pi*2*float64(i)/stepCount) * radius)
		y := (math.Cos(math.Pi*2*float64(i)/stepCount) * radius)

		x2 := (math.Sin(math.Pi*2*float64(i+1)/stepCount) * radius)
		y2 := (math.Cos(math.Pi*2*float64(i+1)/stepCount) * radius)

		ebitenutil.DrawLine(screen, transformedCenter[0]+x, transformedCenter[1]+y, transformedCenter[0]+x2, transformedCenter[1]+y2, drawColor)
	}

}

// DrawDebugText draws render debug information (like number of drawn objects, number of drawn triangles, frame time, etc)
// at the top-left of the provided screen *ebiten.Image, using the textScale and color provided.
func (camera *Camera) DrawDebugText(screen *ebiten.Image, textScale float64, color *Color) {
	dr := &ebiten.DrawImageOptions{}
	dr.ColorM.Scale(color.ToFloat64s())
	dr.GeoM.Translate(0, textScale*16)
	dr.GeoM.Scale(textScale, textScale)

	m := camera.DebugInfo.AvgFrameTime.Round(time.Microsecond).Microseconds()
	ft := fmt.Sprintf("%.2fms", float32(m)/1000)

	m = camera.DebugInfo.AvgAnimationTime.Round(time.Microsecond).Microseconds()
	at := fmt.Sprintf("%.2fms", float32(m)/1000)

	m = camera.DebugInfo.AvgLightTime.Round(time.Microsecond).Microseconds()
	lt := fmt.Sprintf("%.2fms", float32(m)/1000)

	text.DrawWithOptions(screen, fmt.Sprintf(
		"FPS: %f\nTotal render frame-time: %s\nSkinned mesh animation time: %s\nLighting frame-time: %s\nDraw calls: %d/%d\nRendered triangles: %d/%d\nActive Lights: %d/%d",
		ebiten.CurrentFPS(),
		ft,
		at,
		lt,
		camera.DebugInfo.DrawnParts,
		camera.DebugInfo.TotalParts,
		camera.DebugInfo.DrawnTris,
		camera.DebugInfo.TotalTris,
		camera.DebugInfo.ActiveLightCount,
		camera.DebugInfo.LightCount),
		basicfont.Face7x13, dr)

}

// DrawDebugWireframe draws the wireframe triangles of all visible Models underneath the rootNode in the color provided to the screen
// image provided.
func (camera *Camera) DrawDebugWireframe(screen *ebiten.Image, rootNode INode, color *Color) {

	vpMatrix := camera.ViewMatrix().Mult(camera.Projection())

	allModels := append([]INode{rootNode}, rootNode.ChildrenRecursive()...)

	camWidth, camHeight := camera.ColorTexture.Size()

	for _, m := range allModels {

		if model, isModel := m.(*Model); isModel {

			if model.FrustumCulling {

				if !model.BoundingSphere.Intersecting(camera.FrustumSphere) {
					continue
				}

			}

			for _, meshPart := range model.Mesh.MeshParts {

				model.TransformedVertices(vpMatrix, camera, meshPart)

				for _, tri := range meshPart.Triangles {

					v0 := camera.ClipToScreen(tri.Vertices[0].transformed)
					v1 := camera.ClipToScreen(tri.Vertices[1].transformed)
					v2 := camera.ClipToScreen(tri.Vertices[2].transformed)

					if (v0[0] < 0 && v1[0] < 0 && v2[0] < 0) ||
						(v0[1] < 0 && v1[1] < 0 && v2[1] < 0) ||
						(v0[0] > float64(camWidth) && v1[0] > float64(camWidth) && v2[0] > float64(camWidth)) ||
						(v0[1] > float64(camHeight) && v1[1] > float64(camHeight) && v2[1] > float64(camHeight)) {
						continue
					}

					c := color.ToRGBA64()
					ebitenutil.DrawLine(screen, float64(v0[0]), float64(v0[1]), float64(v1[0]), float64(v1[1]), c)
					ebitenutil.DrawLine(screen, float64(v1[0]), float64(v1[1]), float64(v2[0]), float64(v2[1]), c)
					ebitenutil.DrawLine(screen, float64(v2[0]), float64(v2[1]), float64(v0[0]), float64(v0[1]), c)

				}

			}

		}

	}

}

// DrawDebugDrawOrder draws the drawing order of all triangles of all visible Models underneath the rootNode in the color provided to the screen
// image provided.
func (camera *Camera) DrawDebugDrawOrder(screen *ebiten.Image, rootNode INode, textScale float64, color *Color) {

	vpMatrix := camera.ViewMatrix().Mult(camera.Projection())

	allModels := append([]INode{rootNode}, rootNode.ChildrenRecursive()...)

	for _, m := range allModels {

		if model, isModel := m.(*Model); isModel {

			if model.FrustumCulling {

				if !model.BoundingSphere.Intersecting(camera.FrustumSphere) {
					continue
				}

			}

			for _, meshPart := range model.Mesh.MeshParts {

				model.TransformedVertices(vpMatrix, camera, meshPart)

				for triIndex, tri := range meshPart.sortedTriangles {

					screenPos := camera.WorldToScreen(model.Transform().MultVec(tri.Center))

					dr := &ebiten.DrawImageOptions{}
					dr.ColorM.Scale(color.ToFloat64s())
					dr.GeoM.Translate(screenPos[0], screenPos[1]+(textScale*16))
					dr.GeoM.Scale(textScale, textScale)

					text.DrawWithOptions(screen, fmt.Sprintf("%d", triIndex), basicfont.Face7x13, dr)

				}

			}

		}

	}

}

// DrawDebugDrawCallCount draws the draw call count of all visible Models underneath the rootNode in the color provided to the screen
// image provided.
func (camera *Camera) DrawDebugDrawCallCount(screen *ebiten.Image, rootNode INode, textScale float64, color *Color) {

	allModels := append([]INode{rootNode}, rootNode.ChildrenRecursive()...)

	for _, m := range allModels {

		if model, isModel := m.(*Model); isModel {

			if model.FrustumCulling {

				if !model.BoundingSphere.Intersecting(camera.FrustumSphere) {
					continue
				}

			}

			screenPos := camera.WorldToScreen(model.WorldPosition())

			dr := &ebiten.DrawImageOptions{}
			dr.ColorM.Scale(color.ToFloat64s())
			dr.GeoM.Translate(screenPos[0], screenPos[1]+(textScale*16))
			dr.GeoM.Scale(textScale, textScale)

			text.DrawWithOptions(screen, fmt.Sprintf("%d", len(model.Mesh.MeshParts)), basicfont.Face7x13, dr)

		}

	}

}

// DrawDebugNormals draws the normals of visible models underneath the rootNode given to the screen. NormalLength is the length of the normal lines
// in units. Color is the color to draw the normals.
func (camera *Camera) DrawDebugNormals(screen *ebiten.Image, rootNode INode, normalLength float64, color *Color) {

	allModels := append([]INode{rootNode}, rootNode.ChildrenRecursive()...)

	for _, m := range allModels {

		if model, isModel := m.(*Model); isModel {

			if model.FrustumCulling {

				if !model.BoundingSphere.Intersecting(camera.FrustumSphere) {
					continue
				}

			}

			for _, mp := range model.Mesh.MeshParts {

				for _, tri := range mp.Triangles {
					center := camera.WorldToScreen(model.Transform().MultVecW(tri.Center))
					transformedNormal := camera.WorldToScreen(model.Transform().MultVecW(tri.Center.Add(tri.Normal.Scale(normalLength))))
					ebitenutil.DrawLine(screen, center[0], center[1], transformedNormal[0], transformedNormal[1], color.ToRGBA64())
				}

			}

		}

	}

}

// DrawDebugCenters draws the center positions of nodes under the rootNode using the color given to the screen image provided.
func (camera *Camera) DrawDebugCenters(screen *ebiten.Image, rootNode INode, color *Color) {

	allModels := append([]INode{rootNode}, rootNode.ChildrenRecursive()...)

	c := color.ToRGBA64()

	for _, node := range allModels {

		if node == camera {
			continue
		}

		camera.drawCircle(screen, node.WorldPosition(), 8, c)

		// If the node's parent is something, and its parent's parent is something (i.e. it's not the root)
		if node.Parent() != nil && node.Parent() != node.Root() {
			parentPos := camera.WorldToScreen(node.Parent().WorldPosition())
			pos := camera.WorldToScreen(node.WorldPosition())
			ebitenutil.DrawLine(screen, pos[0], pos[1], parentPos[0], parentPos[1], c)
		}

	}

}

// DrawDebugBoundsColored will draw shapes approximating the shapes and positions of BoundingObjects underneath the rootNode. The shapes will
// be drawn in the color provided for each kind of bounding object to the screen image provided.
func (camera *Camera) DrawDebugBoundsColored(screen *ebiten.Image, rootNode INode, aabbColor, sphereColor, capsuleColor, trianglesColor *Color) {

	allModels := append([]INode{rootNode}, rootNode.ChildrenRecursive()...)

	camWidth, camHeight := camera.ColorTexture.Size()

	for _, n := range allModels {

		if b, isBounds := n.(BoundingObject); isBounds {

			switch bounds := b.(type) {

			case *BoundingSphere:

				pos := bounds.WorldPosition()
				radius := bounds.WorldRadius()

				u := camera.WorldToScreen(pos.Add(vector.Y.Scale(radius)))
				d := camera.WorldToScreen(pos.Add(vector.Y.Scale(-radius)))
				r := camera.WorldToScreen(pos.Add(vector.X.Scale(radius)))
				l := camera.WorldToScreen(pos.Add(vector.X.Scale(-radius)))
				f := camera.WorldToScreen(pos.Add(vector.Z.Scale(radius)))
				b := camera.WorldToScreen(pos.Add(vector.Z.Scale(-radius)))

				lines := []vector.Vector{
					u, r, d, l,
					u, f, d, b, u,
					b, r, f, l, b,
				}

				for i := range lines {

					if i >= len(lines)-1 {
						break
					}

					start := lines[i]
					end := lines[i+1]
					ebitenutil.DrawLine(screen, start[0], start[1], end[0], end[1], sphereColor.ToRGBA64())

				}

			case *BoundingCapsule:

				pos := bounds.WorldPosition()
				radius := bounds.WorldRadius()
				height := bounds.Height / 2

				uv := bounds.WorldRotation().Up()
				rv := bounds.WorldRotation().Right()
				fv := bounds.WorldRotation().Forward()

				u := camera.WorldToScreen(pos.Add(uv.Scale(height)))

				ur := camera.WorldToScreen(pos.Add(uv.Scale(height - radius)).Add(rv.Scale(radius)))
				ul := camera.WorldToScreen(pos.Add(uv.Scale(height - radius)).Add(rv.Scale(-radius)))
				uf := camera.WorldToScreen(pos.Add(uv.Scale(height - radius)).Add(fv.Scale(radius)))
				ub := camera.WorldToScreen(pos.Add(uv.Scale(height - radius)).Add(fv.Scale(-radius)))

				d := camera.WorldToScreen(pos.Add(uv.Scale(-height)))

				dr := camera.WorldToScreen(pos.Add(uv.Scale(-(height - radius))).Add(rv.Scale(radius)))
				dl := camera.WorldToScreen(pos.Add(uv.Scale(-(height - radius))).Add(rv.Scale(-radius)))
				df := camera.WorldToScreen(pos.Add(uv.Scale(-(height - radius))).Add(fv.Scale(radius)))
				db := camera.WorldToScreen(pos.Add(uv.Scale(-(height - radius))).Add(fv.Scale(-radius)))

				lines := []vector.Vector{
					u, ur, dr, d, dl, ul,
					u, uf, df, d, db, ub, u,
					ul, uf, ur, ub, ul,
					dl, db, dr, df, dl,
				}

				for i := range lines {

					if i >= len(lines)-1 {
						break
					}

					start := lines[i]
					end := lines[i+1]
					ebitenutil.DrawLine(screen, start[0], start[1], end[0], end[1], capsuleColor.ToRGBA64())

				}

			case *BoundingAABB:

				pos := bounds.WorldPosition()
				size := bounds.Size.Scale(1.0 / 2.0)

				ufr := camera.WorldToScreen(pos.Add(vector.Vector{size[0], size[1], size[2]}))
				ufl := camera.WorldToScreen(pos.Add(vector.Vector{-size[0], size[1], size[2]}))
				ubr := camera.WorldToScreen(pos.Add(vector.Vector{size[0], size[1], -size[2]}))
				ubl := camera.WorldToScreen(pos.Add(vector.Vector{-size[0], size[1], -size[2]}))

				dfr := camera.WorldToScreen(pos.Add(vector.Vector{size[0], -size[1], size[2]}))
				dfl := camera.WorldToScreen(pos.Add(vector.Vector{-size[0], -size[1], size[2]}))
				dbr := camera.WorldToScreen(pos.Add(vector.Vector{size[0], -size[1], -size[2]}))
				dbl := camera.WorldToScreen(pos.Add(vector.Vector{-size[0], -size[1], -size[2]}))

				lines := []vector.Vector{
					ufr, ufl, ubl, ubr, ufr,
					dfr, dfl, dbl, dbr, dfr,
					ufr, ufl, dfl, dbl, ubl, ubr, dbr,
				}

				for i := range lines {

					if i >= len(lines)-1 {
						break
					}

					start := lines[i]
					end := lines[i+1]
					ebitenutil.DrawLine(screen, start[0], start[1], end[0], end[1], aabbColor.ToRGBA64())

				}

			case *BoundingTriangles:

				lines := []vector.Vector{}

				for _, mp := range bounds.Mesh.MeshParts {

					for _, tri := range mp.Triangles {

						mvpMatrix := bounds.Transform().Mult(camera.ViewMatrix().Mult(camera.Projection()))

						v0 := camera.ClipToScreen(mvpMatrix.MultVecW(tri.Vertices[0].Position))
						v1 := camera.ClipToScreen(mvpMatrix.MultVecW(tri.Vertices[1].Position))
						v2 := camera.ClipToScreen(mvpMatrix.MultVecW(tri.Vertices[2].Position))

						if (v0[0] < 0 && v1[0] < 0 && v2[0] < 0) ||
							(v0[1] < 0 && v1[1] < 0 && v2[1] < 0) ||
							(v0[0] > float64(camWidth) && v1[0] > float64(camWidth) && v2[0] > float64(camWidth)) ||
							(v0[1] > float64(camHeight) && v1[1] > float64(camHeight) && v2[1] > float64(camHeight)) {
							continue
						}

						lines = append(lines, v0, v1, v2)

					}

				}

				triColor := trianglesColor.ToRGBA64()

				for i := 0; i < len(lines); i += 3 {

					if i >= len(lines)-1 {
						break
					}

					start := lines[i]
					end := lines[i+1]
					ebitenutil.DrawLine(screen, start[0], start[1], end[0], end[1], triColor)

					start = lines[i+1]
					end = lines[i+2]
					ebitenutil.DrawLine(screen, start[0], start[1], end[0], end[1], triColor)

					start = lines[i+2]
					end = lines[i]
					ebitenutil.DrawLine(screen, start[0], start[1], end[0], end[1], triColor)

				}

				camera.DrawDebugBoundsColored(screen, bounds.BoundingAABB, aabbColor, sphereColor, capsuleColor, trianglesColor)

			}

		}

	}

}

// DrawDebugBounds will draw shapes approximating the shapes and positions of BoundingObjects underneath the rootNode. The shapes will
// be drawn in the color provided to the screen image provided.
func (camera *Camera) DrawDebugBounds(screen *ebiten.Image, rootNode INode, color *Color) {
	camera.DrawDebugBoundsColored(screen, rootNode, color, color, color, color)
}

/////

// AddChildren parents the provided children Nodes to the passed parent Node, inheriting its transformations and being under it in the scenegraph
// hierarchy. If the children are already parented to other Nodes, they are unparented before doing so.
func (camera *Camera) AddChildren(children ...INode) {
	camera.addChildren(camera, children...)
}

// Unparent unparents the Camera from its parent, removing it from the scenegraph.
func (camera *Camera) Unparent() {
	if camera.parent != nil {
		camera.parent.RemoveChildren(camera)
	}
}

// Type returns the NodeType for this object.
func (camera *Camera) Type() NodeType {
	return NodeTypeCamera
}
