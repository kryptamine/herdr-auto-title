# Changelog

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
