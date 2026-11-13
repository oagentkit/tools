// Package bgjob 远程后台作业 marker 协议原语（tools#106）：经宿主**既有** SSH
// 会话在远端起「脱离会话的长任务」，用退出码 marker 文件做探测与收尾。
//
// # 协议（四条实弹坑全部协议化，jumpserver-exec 背书）
//
//  1. 退出码走 marker 文件（输出路径 + ".rc." + 任务身份）：输出文件早就
//     在场，光看它存在与否判断不了「还在跑」——marker 才是终态的唯一凭证。
//     Status 据此区分 running（输出在场、marker 未落）/ finished（marker
//     已落，含退出码）/ removed（双缺，已收尾）。
//  2. 命令在内层 shell 跑、marker 由**外层**写：把 `; echo $? > marker` 拼进
//     用户命令，任务里任何 `exit` 都会让 shell 先死、marker 永远不写，任务
//     看起来永远 RUNNING。Start 的启动命令是双层结构——内层 shell 只跑用户
//     命令，外层 wrapper 在内层死后照常把 `$?` 落进 marker。
//  3. marker 路径单源拼接：marker = 输出路径 + ".rc." + 本次 Start 的任务
//     身份（markerPathOf 在 Go 侧一次算定，Job.MarkerPath），启动/探测/清理
//     命令对它整段单引号包裹（shellQuote 单源）——「.rc 拼进引号内变成
//     p.rc.rc、探测永远 RUNNING」的 P0 活虫在协议层不可能复发；路径含空格/
//     单引号逐字抵达远端（回归测试钉住命令原文与真 shell 行为）。身份进
//     文件名是同路径复用下「在飞旧任务写回」的防线（tools#161）：旧 wrapper
//     的 `echo $?` 落旧路径，新任务的探测/清理只认自己的路径——启动链的
//     rm -f 只清 Start 时刻已落盘的残留，挡不住 Start 之后才执行的迟到写回。
//  4. 清理自证：Cleanup 先探测——RUNNING 拒绝（任务还在写输出，且文件
//     原样）；FINISHED 后 rm -f 输出与 marker，删完 [ -e ] **自证** REMOVED
//     才报「已清理」；REMOVED 幂等（重复收尾无物可清，不报错）。
//
// # 边界（提案正文声明）
//
//   - 不做进程树管理（本地子进程是 procx 的领域）；不做日志轮转——输出
//     重定向目标由调用方经 WithOutputPath 给定（必填、远端绝对路径）；
//   - 启动/探测/清理复用宿主既有会话（Session 小接口：Exec(string)
//     (stdout, code, err)），本包不做连接管理、不做重试——连接层的断连
//     与重连是宿主会话的职责，探测/清理失败如实上抛；
//   - 远端契约：POSIX sh + setsid + nohup（Linux 堡垒机/生产主机缺省满足）。
//
// 以上契约（marker 判据、外层写、路径单源、REMOVED 自证）是本原语的
// **验收项**而非实现细节，回归测试逐条钉死。
package bgjob
