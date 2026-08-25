# Changelog

## [0.2.0](https://github.com/kryptamine/herdr-auto-title/compare/v0.1.0...v0.2.0) (2026-08-25)


### ⚠ BREAKING CHANGES

* **resolver:** HERDR_AUTO_TITLE_BRANCH_MAX is gone, and a tab in a repository no longer carries its branch.

### Features

* **app:** sweep the session while the event stream catches up ([4cfb39c](https://github.com/kryptamine/herdr-auto-title/commit/4cfb39ce4b930cf38bace0ae54158d55dba8e6d4))
* **resolver:** bind a kind to its detail with an angle ([c1ebfdc](https://github.com/kryptamine/herdr-auto-title/commit/c1ebfdcd64f7d4314b00a9afdc3ed30244be1ab5))
* **resolver:** drop a workspace name a tab only repeats ([08f4455](https://github.com/kryptamine/herdr-auto-title/commit/08f4455e2bcd74d8925f73ad8847220ae9d3e0cb))
* **resolver:** drop the git branch source ([2d90d74](https://github.com/kryptamine/herdr-auto-title/commit/2d90d74619ea7cca027f0515fd2146c57caa8f9f))
* **resolver:** name a pane after the program running in it ([4a4653b](https://github.com/kryptamine/herdr-auto-title/commit/4a4653bcc8478f437e833fb779deb28cfad52130))
* **resolver:** name a remote session after its host ([a02acee](https://github.com/kryptamine/herdr-auto-title/commit/a02aceee5c7166a00a2a94b1c7aa35c8de78dd96))
* **resolver:** name tabs after the branch they are on ([6dd1fec](https://github.com/kryptamine/herdr-auto-title/commit/6dd1fec23afa41b7d244ef66029f1841d6afda83))
* **resolver:** title tabs from agent context ([8339aaa](https://github.com/kryptamine/herdr-auto-title/commit/8339aaa7296d4eeb6ec49e113f57353a7548ba0d))
* **resolver:** title tabs from the terminal title ([dbd714b](https://github.com/kryptamine/herdr-auto-title/commit/dbd714b2d462d3c263800ba24dc1f368657aceaa))
* **resolver:** treat an agent as the kind of program it is ([f9ad5ff](https://github.com/kryptamine/herdr-auto-title/commit/f9ad5ffa37565963b16c3dd339db27bccfa9c71e))
* **state:** stop naming a tab the user has renamed ([a566693](https://github.com/kryptamine/herdr-auto-title/commit/a566693406a1e6b1c6b3e7168869fe001685d573))
* title tabs from the pane working directory ([a53677e](https://github.com/kryptamine/herdr-auto-title/commit/a53677e31166feea8283fcb288fbcbac9ab79de1))


### Bug Fixes

* **app:** survive a Herdr that is down, or not up yet ([0ef933d](https://github.com/kryptamine/herdr-auto-title/commit/0ef933d4bda10cd9ed50ac9781f13a11570d5918))
* **resolver:** keep the file name in editor terminal titles ([99b89ed](https://github.com/kryptamine/herdr-auto-title/commit/99b89ed02abab7c28b8e458e1a876573db8e7be9))
* **resolver:** keep the remote mark out of the contested slot ([3bfdee5](https://github.com/kryptamine/herdr-auto-title/commit/3bfdee583421c84787673ef60338429b251aebbd))
* **resolver:** mark the host, not the activity, as remote ([0546296](https://github.com/kryptamine/herdr-auto-title/commit/0546296ac1342d25b36d4fa073e5ad7d43425847))
* **resolver:** measure a title in columns, not runes ([b78ca7c](https://github.com/kryptamine/herdr-auto-title/commit/b78ca7c63f7825cbbdf1a35580b15b2ec724263a))
* **resolver:** reduce a branch to what identifies it ([1c11e69](https://github.com/kryptamine/herdr-auto-title/commit/1c11e694decada3957205560ab2304e0a772834e))
* **resolver:** reject a shell prompt as an activity ([db01fd0](https://github.com/kryptamine/herdr-auto-title/commit/db01fd0f71b3e1be8e5ac910ca8e3bad122f4ed6))
* **state:** drop the pane updates herdr replays on subscribe ([790d0ed](https://github.com/kryptamine/herdr-auto-title/commit/790d0ed9ebe6254abb886db840df83eb0c893bf4))
* **state:** keep a tab named before its first poll ([645ce09](https://github.com/kryptamine/herdr-auto-title/commit/645ce09575aac910e528902a4e244b502530002e))
* **state:** stop losing tabs to manual rename protection ([49fc45f](https://github.com/kryptamine/herdr-auto-title/commit/49fc45fcb847ef641e0dd9f616793c4df0d4294b))


### Refactoring

* **app:** read every setting through one reader ([b85eb10](https://github.com/kryptamine/herdr-auto-title/commit/b85eb10226c7a626176b457f82f0257eed40503d))
* **app:** stop Run claiming an error it cannot return ([5bb32a3](https://github.com/kryptamine/herdr-auto-title/commit/5bb32a30a8e043b96493878f22f20526fcb5fcbf))
* clear the smells the review named ([dd163b3](https://github.com/kryptamine/herdr-auto-title/commit/dd163b3b978bcfc3c3c58b3cac96fc7244419958))
* poll the session instead of subscribing to events ([0ebde31](https://github.com/kryptamine/herdr-auto-title/commit/0ebde319afaddc73c689a3382c892890086b7c93))
* read tab state instead of trusting event payloads ([f50a02a](https://github.com/kryptamine/herdr-auto-title/commit/f50a02ade702a3ed5fa14930db9a1056bc000839))
* **resolver:** build every source through a constructor ([ae7d327](https://github.com/kryptamine/herdr-auto-title/commit/ae7d327684c84322fd330ee12c5efbc4c1c4d97e))
* **resolver:** join every part of a title with one separator ([92f8ab2](https://github.com/kryptamine/herdr-auto-title/commit/92f8ab2f4d435278b8e58aa868f130ab8003af5f))
* **resolver:** let each source state its own place on the ladder ([a913411](https://github.com/kryptamine/herdr-auto-title/commit/a9134110f74b0706b18d682592b4f0460c125645))
* **resolver:** split Resolve into the steps it was hiding ([69dd7ec](https://github.com/kryptamine/herdr-auto-title/commit/69dd7ecaa79a397cf665bbca64941a83253e5f3e))
