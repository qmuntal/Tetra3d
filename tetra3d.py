# Add-on for Tetra3D > Blender exporting

import bpy, os
from bpy.app.handlers import persistent

bl_info = {
    "name" : "Tetra3D Addon",                        # The name in the addon search menu
    "author" : "SolarLune Games",
    "description" : "An addon for exporting GLTF content from Blender for use with Tetra3D.",
    "blender" : (3, 0, 1),                             # Lowest version to use
    "location" : "View3D",
    "category" : "Gamedev",
    "version" : (0, 2),
    "support" : "COMMUNITY",
    "doc_url" : "https://github.com/SolarLune/Tetra3d/wiki/Blender-Addon",
}

boundsTypes = [
    ("NONE", "No Bounds", "No collision will be created for this object.", 0, 0),
    ("AABB", "AABB", "An AABB (axis-aligned bounding box). If the size isn't customized, it will be big enough to fully contain the mesh of the current object. Currently buggy when resolving intersections between AABB or other Triangle Nodes", 0, 1),
    ("CAPSULE", "Capsule", "A capsule, which can rotate. If the radius and height are not set, it will have a radius and height to fully contain the current object", 0, 2),
    ("SPHERE", "Sphere", "A sphere. If the radius is not custom set, it will have a large enough radius to fully contain the provided object", 0, 3),
    ("TRIANGLES", "Triangle Mesh", "A triangle mesh bounds type. Only works on mesh-type objects (i.e. an Empty won't generate a BoundingTriangles). Accurate, but slow. Currently buggy when resolving intersections between AABB or other Triangle Nodes", 0, 4),
]

gltfExportTypes = [
    ("GLB", ".glb", "Exports a single file, with all data packed in binary form. Most efficient and portable, but more difficult to edit later", 0, 0),
    ("GLTF_SEPARATE", ".gltf + .bin + textures", "Exports multiple files, with separate JSON, binary and texture data. Easiest to edit later - Note that Tetra3D doesn't support this properly currently", 0, 1),
    ("GLTF_EMBEDDED", ".gltf", "Exports a single file, with all data packed in JSON. Less efficient than binary, but easier to edit later", 0, 2),
 ]

GamePropTypeBool = 1
GamePropTypeFloat = 2
GamePropTypeString = 3
GamePropTypeLink = 4

gamePropTypes = [
    ("bool", "Bool", "Boolean data type", 0, 0),
    ("int", "Int", "Int data type", 0, 1),
    ("float", "Float", "Float data type", 0, 2),
    ("string", "String", "String data type", 0, 3),
    ("reference", "Object", "Object reference data type; converted to a string composed as follows on export - [SCENE NAME]:[OBJECT NAME]", 0, 4),
]

class t3dGamePropertyItem__(bpy.types.PropertyGroup):

    name: bpy.props.StringProperty(name="Name", default="New Property")
    valueType: bpy.props.EnumProperty(items=gamePropTypes, name="Type")

    valueBool: bpy.props.BoolProperty(name = "", description="The boolean value of the property")
    valueInt: bpy.props.IntProperty(name = "", description="The integer value of the property")
    valueFloat: bpy.props.FloatProperty(name = "", description="The float value of the property")
    valueString: bpy.props.StringProperty(name = "", description="The string value of the property")
    valueReference: bpy.props.PointerProperty(name = "", type=bpy.types.Object, description="The object to reference")
    valueReferenceScene: bpy.props.PointerProperty(name = "", type=bpy.types.Scene, description="The scene to search for an object to reference; if this is blank, all objects from all scenes will appear in the object search field")
    

class OBJECT_OT_tetra3dAddProp(bpy.types.Operator):
    bl_idname = "object.tetra3daddprop"
    bl_label = "Add Game Property"
    bl_description= "Adds a game property to the currently selected object. A game property gets added to an Object's Tags object in Tetra3D"
    bl_options = {'REGISTER', 'UNDO'}

    def execute(self, context):
        context.object.t3dGameProperties__.add()
        return {'FINISHED'}

class OBJECT_OT_tetra3dDeleteProp(bpy.types.Operator):
    bl_idname = "object.tetra3ddeleteprop"
    bl_label = "Delete Game Property"
    bl_description= "Deletes a game property from the currently selected object"
    bl_options = {'REGISTER', 'UNDO'}

    index : bpy.props.IntProperty()

    def execute(self, context):
        context.object.t3dGameProperties__.remove(self.index)
        return {'FINISHED'}

class OBJECT_OT_tetra3dReorderProps(bpy.types.Operator):
    bl_idname = "object.tetra3dreorderprops"
    bl_label = "Re-order Game Property"
    bl_description= "Moves a game property up or down in the list"
    bl_options = {'REGISTER', 'UNDO'}

    index : bpy.props.IntProperty()
    moveUp : bpy.props.BoolProperty()

    def execute(self, context):
        if self.moveUp:
            context.object.t3dGameProperties__.move(self.index, self.index-1)
        else:
            context.object.t3dGameProperties__.move(self.index, self.index+1)
        return {'FINISHED'}

def copyProp(fromProp, toProp):
    toProp.name = fromProp.name
    toProp.valueType = fromProp.valueType
    toProp.valueBool = fromProp.valueBool
    toProp.valueInt = fromProp.valueInt
    toProp.valueFloat = fromProp.valueFloat
    toProp.valueString = fromProp.valueString
    toProp.valueReference = fromProp.valueReference
    toProp.valueReferenceScene = fromProp.valueReferenceScene


class OBJECT_OT_tetra3dCopyProps(bpy.types.Operator):
    bl_idname = "object.tetra3dcopyprops"
    bl_label = "Copy Game Properties"
    bl_description= "Copies game properties from the currently selected object to all other selected objects"
    bl_options = {'REGISTER', 'UNDO'}

    def execute(self, context):

        selected = context.object

        for o in context.selected_objects:
            if o == selected:
                continue
            o.t3dGameProperties__.clear()
            for prop in selected.t3dGameProperties__:
                newProp = o.t3dGameProperties__.add()
                copyProp(prop, newProp)

        return {'FINISHED'}

class OBJECT_OT_tetra3dCopyOneProperty(bpy.types.Operator):
    bl_idname = "object.tetra3dcopyoneproperty"
    bl_label = "Copy Game Property"
    bl_description= "Copies a single game property from the currently selected object to all other selected objects"
    bl_options = {'REGISTER', 'UNDO'}

    index : bpy.props.IntProperty()

    def execute(self, context):

        selected = context.object

        for o in context.selected_objects:
            if o == selected:
                continue
            
            fromProp = selected.t3dGameProperties__[self.index]

            if fromProp.name in o.t3dGameProperties__:
                toProp = o.t3dGameProperties__[fromProp.name]
            else:
                toProp = o.t3dGameProperties__.add()

            copyProp(fromProp, toProp)

        return {'FINISHED'}

class OBJECT_OT_tetra3dClearProps(bpy.types.Operator):
   bl_idname = "object.tetra3dclearprops"
   bl_label = "Clear Game Properties"
   bl_description= "Clears game properties from all currently selected objects"
   bl_options = {'REGISTER', 'UNDO'}

   def execute(self, context):

        obj = context.object

        for o in context.selected_objects:
            o.t3dGameProperties__.clear()

        return {'FINISHED'}


class OBJECT_PT_tetra3d(bpy.types.Panel):
    bl_idname = "OBJECT_PT_tetra3d"
    bl_label = "Tetra3d Object Properties"
    bl_space_type = 'PROPERTIES'
    bl_region_type = 'WINDOW'
    bl_context = "object"

    def draw(self, context):
        row = self.layout.row()
        row.prop(context.object, "t3dVisible__")
        row = self.layout.row()
        row.prop(context.object, "t3dBoundsType__")
        row = self.layout.row()
        
        if context.object.t3dBoundsType__ == 'AABB':
            row.prop(context.object, "t3dAABBCustomEnabled__")
            if context.object.t3dAABBCustomEnabled__:
                row = self.layout.row()
                row.prop(context.object, "t3dAABBCustomSize__")
        elif context.object.t3dBoundsType__ == 'CAPSULE':
            row.prop(context.object, "t3dCapsuleCustomEnabled__")
            if context.object.t3dCapsuleCustomEnabled__:
                row = self.layout.row()
                row.prop(context.object, "t3dCapsuleCustomRadius__")
                row.prop(context.object, "t3dCapsuleCustomHeight__")
        elif context.object.t3dBoundsType__ == 'SPHERE':
            row.prop(context.object, "t3dSphereCustomEnabled__")
            if context.object.t3dSphereCustomEnabled__:
                row = self.layout.row()
                row.prop(context.object, "t3dSphereCustomRadius__")
        row = self.layout.row()
        row.separator()
        row = self.layout.row()
        row.operator("object.tetra3daddprop", text="Add Game Property", icon="PLUS")
        row.operator("object.tetra3dcopyprops", text="Overwrite All Game Properties", icon="COPYDOWN")
        
        for index, prop in enumerate(context.object.t3dGameProperties__):
            box = self.layout.box()
            row = box.row()
            row.prop(prop, "name")
            
            moveUpOptions = row.operator(OBJECT_OT_tetra3dReorderProps.bl_idname, text="", icon="TRIA_UP")
            moveUpOptions.index = index
            moveUpOptions.moveUp = True

            moveDownOptions = row.operator(OBJECT_OT_tetra3dReorderProps.bl_idname, text="", icon="TRIA_DOWN")
            moveDownOptions.index = index
            moveDownOptions.moveUp = False

            copy = row.operator(OBJECT_OT_tetra3dCopyOneProperty.bl_idname, text="", icon="COPYDOWN")
            copy.index = index

            deleteOptions = row.operator(OBJECT_OT_tetra3dDeleteProp.bl_idname, text="", icon="TRASH")
            deleteOptions.index = index

            row = box.row()
            row.prop(prop, "valueType")
            if prop.valueType == "bool":
                row.prop(prop, "valueBool")
            elif prop.valueType == "int":
                row.prop(prop, "valueInt")
            elif prop.valueType == "float":
                row.prop(prop, "valueFloat")
            elif prop.valueType == "string":
                row.prop(prop, "valueString")
            elif prop.valueType == "reference":
                row.prop(prop, "valueReferenceScene")
                if prop.valueReferenceScene != None:
                    row.prop_search(prop, "valueReference", prop.valueReferenceScene, "objects")
                else:
                    row.prop(prop, "valueReference")
        
        row = self.layout.row()
        row.operator("object.tetra3dclearprops", text="Clear All Game Properties", icon="CANCEL")
        
class MATERIAL_PT_tetra3d(bpy.types.Panel):
    bl_idname = "MATERIAL_PT_tetra3d"
    bl_label = "Tetra3d Material Properties"
    bl_space_type = 'PROPERTIES'
    bl_region_type = 'WINDOW'
    bl_context = "material"

    def draw(self, context):
        row = self.layout.row()
        row.prop(context.material, "t3dMaterialColor__")
        # row = self.layout.row()
        # row.prop(context.material, "t3dColorTexture0__")
        # row.operator("image.open")
        row = self.layout.row()
        row.prop(context.material, "t3dMaterialShadeless__")
        row.prop(context.material, "use_backface_culling")
        row = self.layout.row()
        row.prop(context.material, "blend_method")

        
# The idea behind "globalget and set" is that we're setting properties on the first scene (which must exist), and getting any property just returns the first one from that scene
def globalGet(propName):
    if propName in bpy.data.scenes[0]:
        return bpy.data.scenes[0][propName]

def globalSet(propName, value):
    bpy.data.scenes[0][propName] = value

class RENDER_PT_tetra3d(bpy.types.Panel):
    bl_idname = "RENDER_PT_tetra3d"
    bl_label = "Tetra3D Render Properties"
    bl_space_type = "PROPERTIES"
    bl_region_type = "WINDOW"
    bl_context = "render"
    
    def draw(self, context):
        row = self.layout.row()
        row.operator(EXPORT_OT_tetra3d.bl_idname)
        row = self.layout.row()
        row.prop(context.scene, "t3dExportOnSave__")

        row = self.layout.row()
        row.prop(context.scene, "t3dExportFilepath__")
        
        row = self.layout.row()
        row.prop(context.scene, "t3dExportFormat__")
        
        box = self.layout.box()
        box.prop(context.scene, "t3dPackTextures__")
        box.prop(context.scene, "t3dExportCameras__")
        box.prop(context.scene, "t3dExportLights__")


def export():
    scene = bpy.context.scene
        
    blendPath = bpy.context.blend_data.filepath
    if scene.t3dExportFilepath__ != "":
        blendPath = scene.t3dExportFilepath__

    if blendPath == "":
        return False
    
    if scene.t3dExportFormat__ == "GLB":
        ending = ".glb"
    elif scene.t3dExportFormat__ == "GLTF_SEPARATE" or scene.t3dExportFormat__ == "GLTF_EMBEDDED":
        ending = ".gltf"
    
    newPath = os.path.splitext(blendPath)[0] + ending

    for obj in bpy.data.objects:
        cloning = []
        if obj.instance_type == "COLLECTION":
            for o in obj.instance_collection.objects:
                if o.parent == None:
                    cloning.append(o.name)
        if len(cloning) > 0:
            obj["t3dInstanceCollection__"] = cloning

    for action in bpy.data.actions:
        markers = []
        for marker in action.pose_markers:
            markerInfo = {
                "name": marker.name,
                "time": marker.frame / scene.render.fps,
            }
            markers.append(markerInfo)
        if len(markers) > 0:
            action["t3dMarkers__"] = markers
    
    # We force on exporting of Extra values because otherwise, values from Blender would not be able to be exported.
    # export_apply=True to ensure modifiers are applied.
    bpy.ops.export_scene.gltf(
        filepath=newPath, 
        export_format=scene.t3dExportFormat__, 
        export_cameras=scene.t3dExportCameras__, 
        export_lights=scene.t3dExportLights__, 
        export_keep_originals=not scene.t3dPackTextures__,

        export_extras=True,
        export_yup=True,
        export_apply=True,
    )

    for obj in bpy.data.objects:
        if "t3dInstanceCollection__" in obj:
            del(obj["t3dInstanceCollection__"])

    for action in bpy.data.actions:
        if "t3dMarkers__" in action:
            del(action["t3dMarkers__"])

    return True

@persistent
def exportOnSave(dummy):
    
    if globalGet("t3dExportOnSave__"):
        export()


class EXPORT_OT_tetra3d(bpy.types.Operator):
   bl_idname = "export.tetra3dgltf"
   bl_label = "Tetra3D Export"
   bl_description= "Exports to a GLTF file for use in Tetra3D"
   bl_options = {'REGISTER', 'UNDO'}

   def execute(self, context):
        if export():
            self.report({"INFO"}, "Tetra3D GLTF data exported properly.")
        else:
            self.report({"WARNING"}, "Warning: Tetra3D GLTF file could not be exported; please either specify a filepath or save the blend file.")
        return {'FINISHED'}


objectProps = {
    "t3dVisible__" : bpy.props.BoolProperty(name="Visible", description="Whether the object is visible or not when exported to Tetra3D", default=True),
    "t3dBoundsType__" : bpy.props.EnumProperty(items=boundsTypes, name="Bounds", description="What Bounding node type to create and parent to this object"),
    "t3dAABBCustomEnabled__" : bpy.props.BoolProperty(name="Custom AABB Size", description="If enabled, you can manually set the BoundingAABB node's size. If disabled, the AABB's size will be automatically determined by this object's mesh (if it is a mesh; otherwise, no BoundingAABB node will be generated)", default=False),
    "t3dAABBCustomSize__" : bpy.props.FloatVectorProperty(name="Size", description="Width (X), height (Y), and depth (Z) of the BoundingAABB node that will be created", min=0.0, default=[2,2,2]),
    "t3dCapsuleCustomEnabled__" : bpy.props.BoolProperty(name="Custom Capsule Size", description="If enabled, you can manually set the BoundingCapsule node's size properties. If disabled, the Capsule's size will be automatically determined by this object's mesh (if it is a mesh; otherwise, no BoundingCapsule node will be generated)", default=False),
    "t3dCapsuleCustomRadius__" : bpy.props.FloatProperty(name="Radius", description="The radius of the BoundingCapsule node.", min=0.0, default=0.5),
    "t3dCapsuleCustomHeight__" : bpy.props.FloatProperty(name="Height", description="The height of the BoundingCapsule node.", min=0.0, default=2),
    "t3dSphereCustomEnabled__" : bpy.props.BoolProperty(name="Custom Sphere Size", description="If enabled, you can manually set the BoundingSphere node's radius. If disabled, the Sphere's size will be automatically determined by this object's mesh (if it is a mesh; otherwise, no BoundingSphere node will be generated)", default=False),
    "t3dSphereCustomRadius__" : bpy.props.FloatProperty(name="Radius", description="Radius of the BoundingSphere node that will be created", min=0.0, default=1),
}

def getExportOnSave(self):
    s = globalGet("t3dExportOnSave__")
    if s is None:
        s = False
    return s

def setExportOnSave(self, value):
    globalSet("t3dExportOnSave__", value)



def getExportFilepath(self):
    fp = globalGet("t3dExportFilepath__")
    if fp is None:
        fp = ""
    return fp

def setExportFilepath(self, value):
    globalSet("t3dExportFilepath__", value)



def getExportFormat(self):
    f = globalGet("t3dExportFormat__")
    if f is None:
        f = 0
    return f

def setExportFormat(self, value):
    globalSet("t3dExportFormat__", value)



def getExportCameras(self):
    c = globalGet("t3dExportCameras__")
    if c is None:
        c = True
    return c

def setExportCameras(self, value):
    globalSet("t3dExportCameras__", value)



def getExportLights(self):
    l = globalGet("t3dExportLights__")
    if l is None:
        l = True
    return l

def setExportLights(self, value):
    globalSet("t3dExportLights__", value)


def getPackTextures(self):
    l = globalGet("t3dPackTextures__")
    if l is None:
        l = False
    return l

def setPackTextures(self, value):
    globalSet("t3dPackTextures__", value)



def register():
    
    bpy.utils.register_class(OBJECT_PT_tetra3d)
    bpy.utils.register_class(RENDER_PT_tetra3d)
    bpy.utils.register_class(MATERIAL_PT_tetra3d)
    bpy.utils.register_class(OBJECT_OT_tetra3dAddProp)
    bpy.utils.register_class(OBJECT_OT_tetra3dDeleteProp)
    bpy.utils.register_class(OBJECT_OT_tetra3dReorderProps)
    bpy.utils.register_class(OBJECT_OT_tetra3dCopyProps)
    bpy.utils.register_class(OBJECT_OT_tetra3dCopyOneProperty)
    bpy.utils.register_class(OBJECT_OT_tetra3dClearProps)
    bpy.utils.register_class(EXPORT_OT_tetra3d)
    
    bpy.utils.register_class(t3dGamePropertyItem__)

    for propName, prop in objectProps.items():
        setattr(bpy.types.Object, propName, prop)

    bpy.types.Object.t3dGameProperties__ = bpy.props.CollectionProperty(type=t3dGamePropertyItem__)

    bpy.types.Scene.t3dExportOnSave__ = bpy.props.BoolProperty(name="Export on Save", description="Whether the current file should export to GLTF on save or not", default=False, 
    get=getExportOnSave, set=setExportOnSave)
    
    bpy.types.Scene.t3dExportFilepath__ = bpy.props.StringProperty(name="Export Filepath", description="Filepath to export GLTF file. If left blank, it will export to the same directory as the blend file and will have the same filename; in this case, if the blend file has not been saved, nothing will happen", 
    default="", subtype="FILE_PATH", get=getExportFilepath, set=setExportFilepath)
    
    bpy.types.Scene.t3dExportFormat__ = bpy.props.EnumProperty(items=gltfExportTypes, name="Export Format", description="What format to export the file in", default="GLTF_EMBEDDED",
    get=getExportFormat, set=setExportFormat)
    
    bpy.types.Scene.t3dExportCameras__ = bpy.props.BoolProperty(name="Export Cameras", description="Whether Blender should export cameras to the GLTF file", default=True,
    get=getExportCameras, set=setExportCameras)

    bpy.types.Scene.t3dExportLights__ = bpy.props.BoolProperty(name="Export Lights", description="Whether Blender should export lights to the GLTF file", default=True,
    get=getExportLights, set=setExportLights)

    bpy.types.Scene.t3dPackTextures__ = bpy.props.BoolProperty(name="Pack Textures", description="Whether Blender should pack textures into the GLTF file on export", default=False,
    get=getPackTextures, set=setPackTextures)    

    bpy.types.Material.t3dMaterialColor__ = bpy.props.FloatVectorProperty(name="Material Color", description="Material modulation color", default=[1,1,1,1], subtype="COLOR", size=4, step=1, min=0, max=1)
    bpy.types.Material.t3dMaterialShadeless__ = bpy.props.BoolProperty(name="Shadeless", description="Whether lighting should affect this material", default=False)
    
    if not exportOnSave in bpy.app.handlers.save_post:
        bpy.app.handlers.save_post.append(exportOnSave)
    
def unregister():
    bpy.utils.unregister_class(OBJECT_PT_tetra3d)
    bpy.utils.unregister_class(RENDER_PT_tetra3d)
    bpy.utils.unregister_class(MATERIAL_PT_tetra3d)
    bpy.utils.unregister_class(OBJECT_OT_tetra3dAddProp)
    bpy.utils.unregister_class(OBJECT_OT_tetra3dDeleteProp)
    bpy.utils.unregister_class(OBJECT_OT_tetra3dReorderProps)
    bpy.utils.unregister_class(OBJECT_OT_tetra3dCopyProps)
    bpy.utils.unregister_class(OBJECT_OT_tetra3dCopyOneProperty)
    bpy.utils.unregister_class(OBJECT_OT_tetra3dClearProps)
    bpy.utils.unregister_class(EXPORT_OT_tetra3d)
    
    bpy.utils.unregister_class(t3dGamePropertyItem__)
    
    for propName, prop in objectProps.items():
        delattr(bpy.types.Object, propName)

    del bpy.types.Object.t3dGameProperties__

    del bpy.types.Scene.t3dExportOnSave__
    del bpy.types.Scene.t3dExportFilepath__
    del bpy.types.Scene.t3dExportFormat__
    del bpy.types.Scene.t3dExportCameras__
    del bpy.types.Scene.t3dExportLights__
    del bpy.types.Scene.t3dPackTextures__

    del bpy.types.Material.t3dMaterialColor__
    del bpy.types.Material.t3dMaterialShadeless__

    if exportOnSave in bpy.app.handlers.save_post:
        bpy.app.handlers.save_post.remove(exportOnSave)
    

if __name__ == "__main__":
    register()
