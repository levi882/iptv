# IPTV Refresh

面向 OpenWrt 的 IPTV 播放列表刷新工具。它可以从已授权的机顶盒登录流程中获取或复用凭据，访问运营商门户并生成适合局域网播放器使用的频道列表。

主要能力：

- 生成 M3U 播放列表，并支持频道排序、分组和名称匹配。
- 发布 XMLTV 节目单，检测主源是否过期并按顺序切换备用源。
- 匹配并缓存本地台标。
- 生成适配 rtp2httpd 的直播与回看地址。
- 提供 LuCI 配置、手动刷新、定时任务和运行状态页面。

OpenWrt 后端、LuCI 和简体中文安装包可从 [Releases](https://github.com/levi882/iptv/releases) 获取。请按发布页提供的 `SHA256SUMS` 校验下载文件，并在 LuCI 中根据自己的网络和 IPTV 业务填写配置。

## 负责任使用

本项目仅供管理本人有权使用的 IPTV 订阅、网络和设备。请遵守当地法律、运营商协议及内容授权要求，不要分享账号凭据、令牌、抓包文件或包含用户信息的运营商响应。

本项目不提供、存储或销售电视节目和直播源，也不隶属于任何运营商、设备厂商、电视台、节目单或台标服务。配置第三方数据源及使用本工具所产生的责任由使用者承担。

## License

代码与文档采用 [Apache License 2.0](LICENSE)。第三方声明及安全说明见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) 和 [SECURITY.md](SECURITY.md)。
