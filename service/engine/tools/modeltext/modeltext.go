// Package modeltext 提供返回给模型的文本的公共度量与整形。
//
// 工具结果由框架 JSON 序列化后发给模型，序列化会放大文本：控制字符、非法 UTF-8
// 每字节展开成 6 字节转义。因此内联体量必须按序列化后的大小度量，按原始字节数
// 判定会让上限形同虚设；所有要限制内联体量的输出点共用这里的度量与上限。
package modeltext

import (
	"encoding/json"
	"unicode/utf8"
)

// MaxInlineBytes 是单次内联返回给模型的体量上限（按 Size 计），各输出点共用。
const MaxInlineBytes = 64 << 10 // 64 KB

// Size 返回 s 经 JSON 字符串序列化后的大小（含两端引号），即它在模型上下文里
// 实际占据的字节数，与框架序列化工具结果的方式逐字节一致。
func Size(s string) int {
	b, _ := json.Marshal(s) // string 序列化不会返回错误
	return len(b)
}

// TrimPartialRune 去掉末尾被字节窗口劈开的半个 UTF-8 字符。
// 被去掉的部分由调用方在下次读取时重新取回，不丢数据。
func TrimPartialRune(b []byte) []byte {
	// UTF-8 单字符最长 4 字节，从末尾回退 4 次必能找到 rune 起点
	for i := len(b) - 1; i >= 0 && i >= len(b)-4; i-- {
		if !utf8.RuneStart(b[i]) {
			continue
		}
		// 不合法说明这个 rune 被截断了，整段丢弃
		if !utf8.Valid(b[i:]) {
			return b[:i]
		}
		return b
	}
	return b
}
