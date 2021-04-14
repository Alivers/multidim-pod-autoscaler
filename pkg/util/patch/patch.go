package patch

// Operation 对资源的操作类型
// 使用 JsonPatchType 进行描述
// 参考: https://jsonpatch.com/
type Operation string

// JsonPatch 中支持的操作
const (
	Add     Operation = "add"
	Remove  Operation = "remove"
	Replace Operation = "replace"
	Copy    Operation = "copy"
	Move    Operation = "move"
	Test    Operation = "test"
)

// Patch 表示了对资源的操作信息
type Patch struct {
	Op    Operation   `json:"op,inline"`
	Path  string      `json:"path,inline"`
	Value interface{} `json:"value"`
}
