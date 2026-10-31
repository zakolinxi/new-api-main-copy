# GitHub Support 工单草稿：请求清除缓存视图与不可达提交

> 用途：提交至 https://support.github.com/contact （选择 "Sensitive data removal"）
> 本文件不含任何明文敏感值；明文仅通过工单私有渠道补充。

---

## 〇、核对过的事实（重写草稿时逐条实测，非推断）

| 事实 | 实测方式 | 结果 |
|---|---|---|
| 受影响 PR | `git ls-remote origin "refs/pull/*/head"` | **0** |
| 远端 fork | GitHub API `/forks` | **0** |
| 远端 ref | `git ls-remote origin` | 只有 `main` + `HEAD`，**无 tag** |
| LFS 指针 | 历史与当前树均无 | **无孤立 LFS 对象** |
| 旧历史提交数 | `git rev-list 9d59a15` | 36 |
| 新历史提交数 | `git rev-list 629698f` | 37（+1 脱敏提交） |
| 内容变更起点 | 逐提交比对 tree | `6733597` → `2443721` |
| 哈希变更起点 | 逐提交比对 | `58ea00a` → `63eef2e`（**tree 相同**，仅剥签名） |
| 旧历史带 GPG 签名 | `%G?` | 2 个：`58ea00a`、`9d59a15` |
| 新历史带 GPG 签名 | 同上 | 0 个 |
| 新 main 全库脱敏值残留 | `git grep` 精确串 | 域名 0、公网 IP 0、DB 账号 0 |
| 旧提交公网可达 | raw.githubusercontent.com | **HTTP 200**（仍可读取） |

---

## 一、工单信息速查（填表用）

| 字段 | 值 |
|---|---|
| Repository owner / name | `zakolinxi/new-api-main-copy` |
| 仓库可见性 | Public（非 fork，fork 数 0） |
| 受影响 PR 数量 | **0** |
| LFS 孤立对象 | **无** |
| 含敏感数据的文件 | `.codepilot-uploads/1790433391422-BUG-channel-group-empty.md` |
| 旧 `main` HEAD | `9d59a15154f852769bfce1e9fff0bf3680bc50d1` |
| 新 `main` HEAD | `629698fd8af188c32e00bd7dc9b083ed1513e710` |
| First Changed Commit（内容） | `6733597a4979f09dac939cb19bc9841f7d774803` → `2443721e788b4e39c56822d27041ffbf22bd7767` |
| First Changed Commit（哈希） | `58ea00ad5e7295e29a89a969502551dc1efc4697` → `63eef2e96a2d175c80c028375d9281058a60d681` |

### 关于 "First Changed Commit(s)"（重要，防止核对不一致）

文档要求提供 `git-filter-repo` 输出里的 `NOTE: First Changed Commit(s)`。**本次未使用 `git-filter-repo`**（本机未安装），而是用 `git fast-export` / `git fast-import` 手工完成，因此没有 filter-repo 日志。两个候选值都必须解释，否则 Support 核对时会出现矛盾：

- **内容层面**：`6733597` → `2443721`。该提交首次把含敏感值的文件引入仓库，其 tree 发生变化。
- **哈希层面**：`58ea00a` → `63eef2e`。该提交**内容完全相同（tree 一致）**，哈希变化仅因为重写剥离了它的 GPG 签名（旧历史 2 个签名，新历史 0 个）。

请在核对时以 `6733597` → `2443721` 为准。`58ea00a` 的变化是签名剥离的副作用，不是内容改写。

---

## 二、泄露内容的准确描述（重要修正）

该文件的"环境信息"章节（第 288–299 行）记录的是内部排障用的运行手册。**经逐行核实，实际泄露的是：**

| 行 | 泄露项 | 说明 |
|---|---|---|
| 292 | 生产服务器**公网 IPv4** + **HTTP 服务端口** | 该端口是 `--port` 启动参数，**不是 SSH 端口** |
| 294 | 数据库**用户名** | 同处注明"密码见 `/etc/new-api.env`"，该文件在服务器上、仓库外 |
| 299 | 反向代理承载的**业务域名** | `https://new.<域名>` |
| 295 / 298 | 服务路径与配置文件路径 | 加固了上述信息的可利用性 |

**未泄露**：无密码、无私钥、无 API Key、无访问令牌、无 SSH 私钥。（已对文件全文检索 `password` / `secret` / `token` / `ghp_` / `sk-` / `ssh-rsa` / `AKIA`，命中数均为 0。）

**同时说明一个易被误判的点**：该文件第 144、151、293、312 行还出现了 `127.0.0.1` 形式的本机地址与 MySQL 本地端口，这些是通用的 loopback 写法，**不属于敏感数据**，请勿计入。

---

## 三、可直接粘贴的工单正文（英文）

```
Subject: Request to purge cached views and unreachable commits that expose
         production host address and domain (zakolinxi/new-api-main-copy)

Hello GitHub Support,

I own the public repository zakolinxi/new-api-main-copy. I rewrote history and
force-pushed, but the old commits are still directly reachable by SHA and still
serve the exposed values. I am requesting cached-view removal and server-side
garbage collection.

== What was exposed ==

A file committed as part of an internal troubleshooting report,
.codepilot-uploads/1790433391422-BUG-channel-group-empty.md, contains a
runbook section ("environment information") that discloses in plaintext:

  - line 292: the production server's public IPv4 address together with the
    HTTP port the application listens on
  - line 294: the database username, with a note that the password lives in
    /etc/new-api.env on the server, outside this repository
  - line 299: the business domain served by the reverse proxy
  - lines 295 and 298: service and config file paths that make the above
    more directly actionable

No password, private key, API key, or access token was ever committed. I
searched the document for password/secret/token/ghp_/sk-/ssh-rsa/AKIA and found
no occurrences. The document only names the database user and points to where
the password is stored on the server.

To be precise about scope: the same file also contains 127.0.0.1 loopback
addresses and the local MySQL port on lines 144, 151, 293 and 312. Those are
generic local values, not sensitive data, and I am not asking you to remove
anything on their account.

Because no authenticating secret leaked, the usual mitigation of rotating an
affected credential would not resolve this. What is exposed is attack surface
- a reachable host address, a port, a domain, and an account name - and no
rotation makes those un-known. That is why I am asking for removal rather than
only rotating credentials.

== What I have already done ==

1. Rewrote the full history so the file no longer contains those values.
   All 36 commits were rewritten; hashes changed from 58ea00a onward. The
   current default branch main is clean: I scanned every blob reachable from
   main for the exact domain, public IP and database username and got zero
   occurrences of each.
2. The current tree is also clean: in main, the offending lines now read
   | 服务器 | `<SERVER_IP>:<SSH_PORT>` |  (line 292)
   | 数据库账号 | `<DB_USER>` (...) |        (line 294)
   | 反代 | nginx -> `https://<YOUR_DOMAIN>` | (line 299)

   One clarification on line 292, so the placeholder does not mislead you: the
   port that was scrubbed there is the application's HTTP listen port (set via
   the --port argument), not an SSH port. The placeholder text "<SSH_PORT>" was
   a naming mistake made during my scrubbing pass. It is only a placeholder -
   it contains no value - but it is mislabeled. I have prepared a follow-up
   commit that renames it to "<HTTP_PORT>"; that commit is not yet pushed, so
   the value you see upstream in the default branch may still read
   "<SSH_PORT>". It has no bearing on this request either way.
3. Force-pushed with --force-with-lease, pinning the expected old value so the
   update would fail rather than clobber concurrent work. The push succeeded:
     + 9d59a15...629698f main -> main (forced update)
4. Verified no forks and no pull requests exist, so there is no third-party copy
   on GitHub that would retain the data.

== What is still exposed (the reason for this ticket) ==

The old commit is still served by SHA, which is the limitation documented in
"Removing sensitive data from a repository":

  https://raw.githubusercontent.com/zakolinxi/new-api-main-copy/9d59a15154f852769bfce1e9fff0bf3680bc50d1/.codepilot-uploads/1790433391422-BUG-channel-group-empty.md

This returns HTTP 200 and line 292 still contains the server address. The
introducing commit 6733597a4979f09dac939cb19bc9841f7d774803 is likewise still
reachable.

== What I am asking for ==

For zakolinxi/new-api-main-copy:
  - Run a garbage collection on the server to expunge the unreachable objects
    that still contain these values
  - Remove the cached views that serve the old commits by SHA

== Information you asked for ==

  - Owner and repository: zakolinxi/new-api-main-copy
  - Number of affected pull requests: 0
  - First changed commit (content):
        6733597a4979f09dac939cb19bc9841f7d774803
     -> 2443721e788b4e39c56822d27041ffbf22bd7767
    This is the commit that first introduced the file.
  - First changed commit (hash):
        58ea00ad5e7295e29a89a969502551dc1efc4697
     -> 63eef2e96a2d175c80c028375d9281058a60d681
    Its tree is identical; the hash changed only because the commit's GPG
    signature was stripped during the rewrite (2 signed commits before,
    0 after). Please treat 6733597 -> 2443721 as the content-level first
    changed commit.
  - Old main HEAD: 9d59a15154f852769bfce1e9fff0bf3680bc50d1
  - New main HEAD: 629698fd8af188c32e00bd7dc9b083ed1513e710
  - LFS objects orphaned by this rewrite: none

Note: I did not use git-filter-repo for this rewrite (it is not installed in my
environment); I used git fast-export / git fast-import, so there is no
filter-repo "First Changed Commit(s)" note to quote. The hashes above are the
verified equivalents.

I have not pasted the plaintext values into this ticket. I am happy to supply
them, or a pre-rewrite bundle of the old history, through a private channel if
needed for verification.

Thank you for your help.
```

---

## 四、中文说明版（备查）

**主题**：请求清除暴露生产主机地址与域名的缓存视图及不可达提交

我是公开仓库 `zakolinxi/new-api-main-copy` 的所有者。已完成历史重写并强制推送，但旧提交仍可按 SHA 直接访问且仍返回未脱敏内容，因此请求清除缓存视图并执行服务端垃圾回收。

**泄露内容**（文件 `.codepilot-uploads/1790433391422-BUG-channel-group-empty.md`"环境信息"章节）：第 292 行生产服务器公网 IPv4 与应用 HTTP 端口（非 SSH 端口）；第 294 行数据库用户名，并注明密码在服务器 `/etc/new-api.env`；第 299 行反代承载的业务域名；第 295、298 行的服务与配置路径，提升了上述信息的可利用性。

**未泄露**：无密码、私钥、API Key、访问令牌。已对全文检索 `password`/`secret`/`token`/`ghp_`/`sk-`/`ssh-rsa`/`AKIA`，命中均为 0。

**范围澄清**：同文件第 144、151、293、312 行的 `127.0.0.1` 与本地 MySQL 端口属通用 loopback 写法，非敏感数据，不请求处理。

**为何不能只靠轮换**：本次未泄露任何认证凭据，轮换无法让已公开的主机地址、端口、域名、账号名重新变为未知。暴露的是攻击面而非认证秘密。

**已完成**：重写全部 36 个提交；当前 tree 已脱敏（第 292/294/299 行现为占位符）；`--force-with-lease` 强推成功（`+ 9d59a15...629698f main -> main`）；`main` 可达全部 blob 精确串扫描，域名/公网 IP/DB 账号命中均为 0；无 fork、无 PR。

**仍存在**：旧提交 `9d59a15` 的 raw 地址仍返回 HTTP 200，第 292 行仍含服务器地址；引入提交 `6733597` 同样可达。

**请求**：服务端 GC 清除含上述值的不可达对象；移除按 SHA 提供旧提交的缓存视图。

---

## 五、⚠️ 关于官方文档的 `git push --force --mirror origin`（不要执行）

官方文档第 2 步要求 `git push --force --mirror origin`。**在本仓库上执行这条命令会把敏感数据重新公开。**

实测本仓库 ref 与远端：

```
629698fd8  refs/heads/main                      ← 远端已有，且干净
629698fd8  refs/remotes/origin/HEAD
629698fd8  refs/remotes/origin/main
a90dc88a5  refs/tags/backup/before-rewrite      ← 指向旧污染历史 9d59a15
```

`--mirror` 会推送**所有** ref，包括标签。实测确认：

- `backup/before-rewrite` → 解析到 `9d59a15154f852769bfce1e9fff0bf3680bc50d1`（**等于旧 main HEAD**）
- 该提交的树**包含**含敏感数据的文件

也就是说，一条 `--mirror` 会把刚清掉的旧历史以 tag 形式推上远端，比不清理更糟。远端目前**无任何 tag**（`git ls-remote --tags origin` 返回空），这才是正确状态。

**结论：本次不要执行 `--mirror`。** 本仓库只有一个分支 `main`，且已用 `--force-with-lease` 单独推送完成（实测远端 `main` = `629698f`，已确认）。已覆盖文档第 2 步的目的，无需再用 `--mirror`。

**将来若确实需要 `--mirror`**，必须先删备份标签：

```
git tag -d backup/before-rewrite
```

但**在 GitHub Support 处理完之前不要删**——它是目前唯一的完整旧历史备份（`.git/backup-before-rewrite.bundle` 同样如此）。

### 与官方流程的其它差异（供核对，不影响受理）

| 文档要求 | 本次实际 | 影响 |
|---|---|---|
| `git filter-repo --sensitive-data-removal` | 环境未安装，改用 `fast-export`/`fast-import` | 无 filter-repo 日志；已给出等价哈希 |
| 从 changed-refs 数受影响 PR | 改用 `git ls-remote origin "refs/pull/*/head"` | 结果同为 **0** |
| 在**新鲜克隆**中操作 | 在含备份的本地仓库中操作 | 本地仍留有旧对象（`git cat-file -t 9d59a15` 返回 commit）。**仅本地**，远端不含 |
| 剥离提交签名 | 同样剥离（旧 2 个签名 → 新 0 个） | **与 filter-repo 行为一致**，非本次操作缺陷 |
| LFS 孤立对象需上传文件 | 仓库无 LFS | 不适用 |

关于签名：文档明确记载「git-filter-repo will remove commit signatures ... for commits that pre-date the sensitive data removal as well」。因此 `58ea00a` 内容未变而哈希变更，是这类工具的**预期行为**，已如实写入工单正文。

---

## 六、提交前自检清单

- [ ] 仓库填 `zakolinxi/new-api-main-copy`
- [ ] 受影响 PR 数填 **0**
- [ ] 首个变更提交填 `6733597` → `2443721`，并说明 `58ea00a` → `63eef2e` 仅为签名剥离（tree 相同）
- [ ] 声明**无 LFS 孤立对象**
- [ ] 声明**未使用 git-filter-repo**，并给出等价哈希
- [ ] 泄露项表述为"公网 IP + HTTP 端口"，**不要写 SSH 端口**
- [ ] 声明未泄露密码等凭据，并说明为何轮换不足以缓解
- [ ] 声明 loopback 地址不在请求范围内
- [ ] 未粘贴任何明文值

> **受理关键**：GitHub 仅在"风险无法通过轮换受影响凭据缓解"时协助清除。
> 本工单论点：泄露的是攻击面而非认证秘密，轮换无法消除。提交时请勿删减该段。

---

## 七、提交工单前建议先做的事

1. **轮换**：数据库密码、SSH 凭据（即便密码未入库，IP 与端口已公开）。
2. **查日志**：服务器 SSH / MySQL 登录记录、`new-api` 访问日志、GitHub 仓库 Traffic。
3. **加 `.gitignore`**：`.codepilot-uploads/` 目前被跟踪（当前 tree 仍含该目录两个文件）。建议后续取消跟踪并加入忽略规则，避免再次误提交。
4. **保留备份**：`.git/backup-before-rewrite.bundle`（8.9 MB）与标签 `backup/before-rewrite` 在工单处理完之前不要删。

---

## 八、参考

- GitHub 官方文档《Removing sensitive data from a repository》
  https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/removing-sensitive-data-from-a-repository
- 工单入口
  https://support.github.com/contact
