# Changelog

## [0.9.0](https://github.com/kryptamine/herdr-auto-title/compare/v0.8.0...v0.9.0) (2026-09-22)


### Features

* **app:** name the workspace row when asked ([c3c08bc](https://github.com/kryptamine/herdr-auto-title/commit/c3c08bc662a89bc345668c836a599aefe429c066))
* **claude:** search every configured Claude config home ([f306002](https://github.com/kryptamine/herdr-auto-title/commit/f306002aa72c879533840ca13689be840e212d04))
* **herdr:** add workspace.rename to the client ([3669825](https://github.com/kryptamine/herdr-auto-title/commit/36698258e86df25694829107828b5d31948b6fff))
* **resolver:** name a workspace after the tab it holds ([c24c8c8](https://github.com/kryptamine/herdr-auto-title/commit/c24c8c842e044f47ee3d8f2abd1416a02286830c))
* **state:** claim a workspace the user named, on sight ([2a27dea](https://github.com/kryptamine/herdr-auto-title/commit/2a27dea62c100138f14fa6e76d26161e5200bee8))


### Bug Fixes

* **resolver:** treat "Windows PowerShell" as a generic title ([666f7bc](https://github.com/kryptamine/herdr-auto-title/commit/666f7bc4bd2a9dfb0704c361369f421148bd05a3))
* **state:** read a drive's root as no directory at all ([7d07b90](https://github.com/kryptamine/herdr-auto-title/commit/7d07b90fa27062cc4a5ebbff16906400be572368))

## [0.8.0](https://github.com/kryptamine/herdr-auto-title/compare/v0.7.0...v0.8.0) (2026-09-20)


### Features

* restart the plugin without a server stop ([50c74d4](https://github.com/kryptamine/herdr-auto-title/commit/50c74d412f0482b7e04ec57909887a077a32d9a7))


### Bug Fixes

* **instance:** accept an instance a newer restart displaced ([c731966](https://github.com/kryptamine/herdr-auto-title/commit/c73196613f7452d22993769fe46193f7d6bb276b))
* **instance:** pass a restart that finished as its deadline fired ([5d5aaac](https://github.com/kryptamine/herdr-auto-title/commit/5d5aaac259709bfa663ff79d83c7a46f9081735c))

## [0.7.0](https://github.com/kryptamine/herdr-auto-title/compare/v0.6.2...v0.7.0) (2026-09-18)


### Features

* **app:** honour XDG_CONFIG_HOME and ~/.config ([f7b27fb](https://github.com/kryptamine/herdr-auto-title/commit/f7b27fbb50556c43486c7c334ba232c9c8c47c3b))
* **app:** look in ~/.config on Windows too ([4fc422a](https://github.com/kryptamine/herdr-auto-title/commit/4fc422a0f6ba37e737cc3048927fb16783221fdc))


### Bug Fixes

* **app:** create a new file in the platform directory ([0533572](https://github.com/kryptamine/herdr-auto-title/commit/05335722dbb53c2ea79cff7e8234149961db33d7))
* **resolver:** follow the agent from a pane holding no repository ([5d8205d](https://github.com/kryptamine/herdr-auto-title/commit/5d8205d740880cec056a08b54d1592dc67d0b902))
* **resolver:** restore the unrecorded-trunk suppression ([7bb7141](https://github.com/kryptamine/herdr-auto-title/commit/7bb7141da629d70413ce3b55b46189a3983bc95d))
* **resolver:** suppress an unrecorded trunk ([960b64a](https://github.com/kryptamine/herdr-auto-title/commit/960b64ae2fdfe79dd43d3e63e16a34ffd25f70c5))

## [0.6.2](https://github.com/kryptamine/herdr-auto-title/compare/v0.6.1...v0.6.2) (2026-09-16)


### Bug Fixes

* **app:** read the branch from the agent's own directory ([c26bd9c](https://github.com/kryptamine/herdr-auto-title/commit/c26bd9c14d34a2578628362c5290ef7db3a7e678))
* **app:** refuse an agent directory that is no longer there ([f4bf2eb](https://github.com/kryptamine/herdr-auto-title/commit/f4bf2ebc6f7aba0dd2e579f3478bf696d2b25ddf))
* **resolver:** pick the branch after labelling both checkouts ([cb04b32](https://github.com/kryptamine/herdr-auto-title/commit/cb04b32a2be750835bda6d9831b6340cc2ad53a1))
* **review:** assert what two tests were named for ([e7b5356](https://github.com/kryptamine/herdr-auto-title/commit/e7b5356f780a2ee065a62896a6694cfc3f3578ef))


### Refactoring

* **claude:** report the cwd as the transcript spelled it ([a04e001](https://github.com/kryptamine/herdr-auto-title/commit/a04e00113d5b0d4a48b265d43f281c06df3dbb05))
* **git:** refuse a directory that is gone where it is read ([0be2799](https://github.com/kryptamine/herdr-auto-title/commit/0be2799f5a889e1131d67ef7e4a36c391ba2fbaa))

## [0.6.1](https://github.com/kryptamine/herdr-auto-title/compare/v0.6.0...v0.6.1) (2026-09-15)


### Bug Fixes

* **app:** keep only a rename that got no answer as possibly landed ([eb59f5d](https://github.com/kryptamine/herdr-auto-title/commit/eb59f5dfbd2fa25c0574e8af370f45057551c7c8))
* **herdr:** keep only a rename that was sent as possibly landed ([cf62093](https://github.com/kryptamine/herdr-auto-title/commit/cf620931869a46a22288aa9b1c7cfafedcab556b))
* **resolver:** drop the local ssh command title while connecting ([95a8de5](https://github.com/kryptamine/herdr-auto-title/commit/95a8de5e86b126491c440864c1743261fc3bd687))
* **resolver:** mark a pane remote only when ssh is in the foreground ([219400c](https://github.com/kryptamine/herdr-auto-title/commit/219400c206ad920ded9fa4f54f16d6c026c91ba8))
* stop a rename that lands after its call failed locking the tab ([8dd8fc2](https://github.com/kryptamine/herdr-auto-title/commit/8dd8fc226d50dc1a6a822b71e7ca478ae6cc05fa))


### Refactoring

* **state:** keep a failed rename's labels with the seen one ([33a88c6](https://github.com/kryptamine/herdr-auto-title/commit/33a88c6348ce975a503ada83db1e5a5e12910d41))

## [0.6.0](https://github.com/kryptamine/herdr-auto-title/compare/v0.5.0...v0.6.0) (2026-09-13)


### ⚠ BREAKING CHANGES

* **app:** Auto Title now names every pane as well as every tab, changing how panes are listed in Herdr's goto panel. The first start renames panes you labelled by hand before, since nothing marks a label as yours until Auto Title has watched it; a pane renamed after that is left alone. Set HERDR_AUTO_TITLE_PANES=false to keep the previous behaviour.

### Features

* **app:** name panes as well as tabs ([7e1f18c](https://github.com/kryptamine/herdr-auto-title/commit/7e1f18c8928442953a20b4375b49ad3b64318d34))
* **app:** name panes by default ([901c0d6](https://github.com/kryptamine/herdr-auto-title/commit/901c0d6d5f4ba3a6fbc18db8ba3e26a08fa182e9))
* HERDR_AUTO_TITLE_PREFER_AGENT names a split tab after its agent pane ([82ed493](https://github.com/kryptamine/herdr-auto-title/commit/82ed493c314a48d135be52ad2029944725cb28b7))


### Bug Fixes

* **resolver:** do not repeat a branch its worktree is named after ([7fcec81](https://github.com/kryptamine/herdr-auto-title/commit/7fcec810280696b8b38b5bd884be2b340c73e02b))


### Refactoring

* name tabs and panes through one path ([a491b66](https://github.com/kryptamine/herdr-auto-title/commit/a491b661812033aa3f4002ed460d05d7b3655245))
* **state:** name the context pane rule and drop its nil case ([7dbcccd](https://github.com/kryptamine/herdr-auto-title/commit/7dbcccd91337325a5701f8489df73513e4b69f40))
* **state:** pick a tab's context pane once, when it is built ([3946461](https://github.com/kryptamine/herdr-auto-title/commit/394646179c21e5c02229c9c0471484c2130bd3fd))

## [0.5.0](https://github.com/kryptamine/herdr-auto-title/compare/v0.4.0...v0.5.0) (2026-09-08)


### Features

* claim windows in the plugin manifest ([4a32a59](https://github.com/kryptamine/herdr-auto-title/commit/4a32a59a4406ef23d8acdf1a4eec04335ccbde39))
* **herdr:** reach the socket through a named pipe on Windows ([4addde4](https://github.com/kryptamine/herdr-auto-title/commit/4addde4c2c9e9c242033991d91016a1445dd307f))
* **resolver:** read what Herdr reports for a Windows pane ([f5bcb5d](https://github.com/kryptamine/herdr-auto-title/commit/f5bcb5d83b074cd3c9bfc56efbbb5c72aaf9b1dd))


### Bug Fixes

* leave when another server takes the socket ([e84aff9](https://github.com/kryptamine/herdr-auto-title/commit/e84aff945e0e196a168383ef47cff1fe4cc03878))


### Refactoring

* **app:** give a pane's reads a module of their own ([2a9516a](https://github.com/kryptamine/herdr-auto-title/commit/2a9516aa2eb7c29c785c0564a60cdf96dbea5660))
* **app:** keep only the interval the loop reads ([73d59da](https://github.com/kryptamine/herdr-auto-title/commit/73d59da533bd1ef30b696326be5d7a86fe06274a))
* **state:** clean a pane's directory where it enters ([2811dda](https://github.com/kryptamine/herdr-auto-title/commit/2811ddaa3d20326dd5c3867b32bfc4e8d6663546))

## [0.4.0](https://github.com/kryptamine/herdr-auto-title/compare/v0.3.3...v0.4.0) (2026-09-05)


### Features

* **app:** let the agent name be turned off ([f72b03f](https://github.com/kryptamine/herdr-auto-title/commit/f72b03f106e28173f6e15c10e2d91cfd034ad994))


### Bug Fixes

* **resolver:** leave a tunnel-only ssh out of the tab's context ([f326333](https://github.com/kryptamine/herdr-auto-title/commit/f3263339cf68a03693dcf321b0fbc430ffe01545))
* **resolver:** stop an agent tab repeating its own directory ([d7eec20](https://github.com/kryptamine/herdr-auto-title/commit/d7eec20ad3241eca03d0c2031c799b2beb40cbf5))


### Refactoring

* **resolver:** build a resolver in one step ([5d29cb3](https://github.com/kryptamine/herdr-auto-title/commit/5d29cb360e666af390f6812480c2a66eedc56739))
* **resolver:** format the agent's name as the part it is ([2d0bc76](https://github.com/kryptamine/herdr-auto-title/commit/2d0bc76e2e526351ac39af44d06abeb1d7ca73ef))
* **resolver:** place a pane's kind in one function ([d84bacb](https://github.com/kryptamine/herdr-auto-title/commit/d84bacbbd18a42829882a99e7b70dc575427e36f))
* **resolver:** trim the tunnel comments ([2906e44](https://github.com/kryptamine/herdr-auto-title/commit/2906e44089775c73dadde9671d24766926c097ad))

## [0.3.3](https://github.com/kryptamine/herdr-auto-title/compare/v0.3.2...v0.3.3) (2026-09-02)


### Bug Fixes

* **herdr:** name a pane after the directory in front of it ([743bb2f](https://github.com/kryptamine/herdr-auto-title/commit/743bb2f9c8b7e70cdeb0cbb998ed343acb77690a))
* **state:** keep the manual lock file to the user who owns it ([5c12576](https://github.com/kryptamine/herdr-auto-title/commit/5c12576dcf47e1baf986b029bbbf74a81ecf6fd9))
* **state:** name a pane after its foreground process's directory ([b5fd498](https://github.com/kryptamine/herdr-auto-title/commit/b5fd49879615ee5599650a1ee9067074d80fb289))


### Refactoring

* **state:** fold the process read back into Read ([8e5ca75](https://github.com/kryptamine/herdr-auto-title/commit/8e5ca75deaaa7352a3cf97b7ec00033c53a013b2))

## [0.3.2](https://github.com/kryptamine/herdr-auto-title/compare/v0.3.1...v0.3.2) (2026-08-28)


### Bug Fixes

* **resolver:** decline an echoed agent name whatever reports it ([f7629b3](https://github.com/kryptamine/herdr-auto-title/commit/f7629b31480ef745e9ff98f0e52ee0e6bdb94aa3))


### Performance

* **app:** read a checkout once per directory per poll ([ce55d9b](https://github.com/kryptamine/herdr-auto-title/commit/ce55d9b8d69683c3b4aba704ab18a3ddb5990813))
* **resolver:** fit a numbered title instead of sanitizing it again ([1fd0d8e](https://github.com/kryptamine/herdr-auto-title/commit/1fd0d8e3e26c40872b7fefbee6e23f9d9a6b185d))
* **resolver:** guard the regexp passes in Sanitize ([9652edc](https://github.com/kryptamine/herdr-auto-title/commit/9652edc667d65270802cea83bde8db30af60f68a))


### Refactoring

* **app:** read both counts through one bound ([f3d73d0](https://github.com/kryptamine/herdr-auto-title/commit/f3d73d0ba13c98f097aa478aea2b35805b375aa1))
* **git:** drop the result nobody outside the tests read ([565d6c9](https://github.com/kryptamine/herdr-auto-title/commit/565d6c9a543d39cc1348c8abdf44aa05f3c356a0))
* **herdr:** drop the agent statuses nothing reads ([c931b58](https://github.com/kryptamine/herdr-auto-title/commit/c931b58914ae0a68c1725d2914da62e34e163966))
* **herdr:** move the test client out of the shipped package ([84da050](https://github.com/kryptamine/herdr-auto-title/commit/84da05074082401ffac68d5d348fb43fdb7b2f2e))
* **resolver:** decline a tab with no panes once, not in every source ([e70c24c](https://github.com/kryptamine/herdr-auto-title/commit/e70c24c0523936df43f0cb66c513be00c7a03c54))
* **resolver:** fold the shared activity shape into one helper ([1bc06b2](https://github.com/kryptamine/herdr-auto-title/commit/1bc06b27f2383514cddc9f94944347db9d8f62ab))

## [0.3.1](https://github.com/kryptamine/herdr-auto-title/compare/v0.3.0...v0.3.1) (2026-08-26)


### Bug Fixes

* **state:** hand a tab back when its name is cleared ([e1aec6f](https://github.com/kryptamine/herdr-auto-title/commit/e1aec6fbf34dbd17da59162dc60df66a034528cf))


### Performance

* **app:** read only the pane that names a tab ([3b9319e](https://github.com/kryptamine/herdr-auto-title/commit/3b9319ed1386a7dec334a1c6dd8741c3c79adfa9))


### Refactoring

* **app:** split the poll loop from its reads ([bd339be](https://github.com/kryptamine/herdr-auto-title/commit/bd339be928b2d9ae953c001e692ae48553b01b75))

## [0.3.0](https://github.com/kryptamine/herdr-auto-title/compare/v0.2.0...v0.3.0) (2026-08-26)


### Features

* **app:** read settings from a configuration file ([#19](https://github.com/kryptamine/herdr-auto-title/issues/19)) ([53e2d17](https://github.com/kryptamine/herdr-auto-title/commit/53e2d17ddcdf15887322bc2a53624f3e28768411))
* cap a generated title at 50 columns ([#17](https://github.com/kryptamine/herdr-auto-title/issues/17)) ([1bf9ede](https://github.com/kryptamine/herdr-auto-title/commit/1bf9ede014c5884af280cc5978dc8663bce7d3e2))

## [0.2.0](https://github.com/kryptamine/herdr-auto-title/compare/v0.1.1...v0.2.0) (2026-08-26)


### Features

* **app:** make the branch width configurable ([76914d1](https://github.com/kryptamine/herdr-auto-title/commit/76914d1374e120a99e2e92420a74c8951d56bedb))
* **git:** read what a repository has checked out ([4d8e0ce](https://github.com/kryptamine/herdr-auto-title/commit/4d8e0ce7069a1c333b7d17d15f2ae7fd99f232d4))
* name a tab after the branch it is on ([96a2c83](https://github.com/kryptamine/herdr-auto-title/commit/96a2c836daa219f28669bd5f902a2965abcb2c63))
* name an agent tab from the session its agent holds ([#16](https://github.com/kryptamine/herdr-auto-title/issues/16)) ([4246532](https://github.com/kryptamine/herdr-auto-title/commit/42465329bf44fef2af5f1f2bd06c9dec6e74a4ba))
* **resolver:** put each tab's position in front of its title ([81112ce](https://github.com/kryptamine/herdr-auto-title/commit/81112ce1efc6583e348bc0da8791b49e8a324b07))


### Bug Fixes

* **app:** stop reading .git when branches are switched off ([7d0c80e](https://github.com/kryptamine/herdr-auto-title/commit/7d0c80eed0286bc30a3746743bfa4e4e21e409b1))
* **resolver:** let a branch that fits keep its whole name ([cc712b8](https://github.com/kryptamine/herdr-auto-title/commit/cc712b8f5616122777cc7050e5ee73786394f7b9))
* **resolver:** match the trunk as git stores it ([d5a4ba2](https://github.com/kryptamine/herdr-auto-title/commit/d5a4ba2d484c1da14318b2baeed83c034e5ff05e))


### Refactoring

* choose a pane's directory in one place ([a78e47c](https://github.com/kryptamine/herdr-auto-title/commit/a78e47ce1ce884e5ff795894f1d6e6669008dc7a))
* cut the comments that outgrew the three-line cap ([97c1746](https://github.com/kryptamine/herdr-auto-title/commit/97c1746794ace658f11a4c528b7f470a8fa2b33b))
* **resolver:** let the branch source own how it labels a checkout ([74e7af3](https://github.com/kryptamine/herdr-auto-title/commit/74e7af3492a8f2a6c7fd25582c733d2438b0c3c2))
* **state:** keep a tab's position as the number it is ([b0f23ae](https://github.com/kryptamine/herdr-auto-title/commit/b0f23ae7c3cbb95b1eca6d4705f774f56c159061))

## [0.1.1](https://github.com/kryptamine/herdr-auto-title/compare/v0.1.0...v0.1.1) (2026-08-25)


### Bug Fixes

* **resolver:** cut a kind by the length that actually matched ([99361ab](https://github.com/kryptamine/herdr-auto-title/commit/99361ab712149c6e7a308591d90092e8746eaae7))
* **resolver:** strip the invisible characters that forge a label ([9e96e14](https://github.com/kryptamine/herdr-auto-title/commit/9e96e14d85ba462b3a8e6262e19636d5e0b323a1))


### Performance

* **app:** reuse a process read instead of making one per pane per poll ([d2595a7](https://github.com/kryptamine/herdr-auto-title/commit/d2595a786ed6df10029ecf3b8243ba2d1ae87032))


### Refactoring

* **herdr:** drop the snapshot fields nothing reads ([a3de7d5](https://github.com/kryptamine/herdr-auto-title/commit/a3de7d51918b7c73be61797c3ed1ac5c608cfcfb))
* **resolver:** make every source decline a nil pane alike ([2f2a036](https://github.com/kryptamine/herdr-auto-title/commit/2f2a03686be02f91579f73bf4158b462446011e2))
* sort through slices rather than sort ([edda7fa](https://github.com/kryptamine/herdr-auto-title/commit/edda7fa675efacc05f1448a81b1318aa929f5691))
* **state:** let encoding/json sort the manual-name file ([75cbf5c](https://github.com/kryptamine/herdr-auto-title/commit/75cbf5c62e51ceb2a92e50593486743fd5b4b6ce))
