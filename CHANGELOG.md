# Changelog

## [0.2.0](https://github.com/kryptamine/herdr-auto-title/compare/v0.1.1...v0.2.0) (2026-08-26)


### Features

* **app:** make the branch width configurable ([76914d1](https://github.com/kryptamine/herdr-auto-title/commit/76914d1374e120a99e2e92420a74c8951d56bedb))
* **git:** read what a repository has checked out ([4d8e0ce](https://github.com/kryptamine/herdr-auto-title/commit/4d8e0ce7069a1c333b7d17d15f2ae7fd99f232d4))
* name a tab after the branch it is on ([dde7af3](https://github.com/kryptamine/herdr-auto-title/commit/dde7af33eaa9e1db6a2309659f543d8eb29846ef))
* name a tab after the branch it is on ([96a2c83](https://github.com/kryptamine/herdr-auto-title/commit/96a2c836daa219f28669bd5f902a2965abcb2c63))
* name an agent tab from the session its agent holds ([#16](https://github.com/kryptamine/herdr-auto-title/issues/16)) ([4246532](https://github.com/kryptamine/herdr-auto-title/commit/42465329bf44fef2af5f1f2bd06c9dec6e74a4ba))
* put each tab's position in front of its title ([e517ee1](https://github.com/kryptamine/herdr-auto-title/commit/e517ee1ff92d62cef2ca52ae41ba6837a87a1db4))
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
