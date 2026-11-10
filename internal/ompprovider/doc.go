// Package ompprovider 实现 PI Agent（oh-my-pi）配置文件的纯文档级读写。
//
// 本包只做三件事：解析 models.yml / models.json / config.yml、按 yaml.Node 做节点级
// 同步（保留注释、键顺序与自定义标签）、按 omp 的 schema 规则做写前校验。
// 它不读取真实用户文件、不执行 !command 引用、不依赖 internal/core。
package ompprovider
