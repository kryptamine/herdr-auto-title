#!/usr/bin/env python3
"""Check the CI gate checker against workflows written to break it."""

import textwrap
import unittest

import check_ci

GOOD = """\
name: CI

on:
  pull_request:

jobs:
  check:
    name: Go ${{ matrix.go-version }}
    strategy:
      matrix:
        include:
          - { os: ubuntu-latest }
    steps:
      - run: go test ./...

  dist-run:
    needs: dist
    steps:
      - run: true

  # A comment at job indent is not a job.
  dist:
    steps:
      - run: true

  gate:
    name: CI passed
    needs: [check, dist, dist-run]
    if: always()
    steps:
      - run: true
"""


def problems(text: str) -> list[str]:
    return check_ci.problems(textwrap.dedent(text))


class CheckCiTest(unittest.TestCase):
    def test_a_complete_gate_passes(self) -> None:
        self.assertEqual(problems(GOOD), [])

    def test_a_job_left_out_of_the_gate_is_named(self) -> None:
        text = GOOD.replace("needs: [check, dist, dist-run]", "needs: [check, dist]")
        self.assertEqual(problems(text), ["the gate does not need dist-run"])

    def test_a_need_naming_no_job_is_named(self) -> None:
        text = GOOD.replace("needs: [check, dist, dist-run]", "needs: [check, dist, dist-run, lint]")
        self.assertEqual(problems(text), ["the gate needs lint, which is not a job"])

    def test_a_workflow_without_the_gate_fails(self) -> None:
        text = GOOD.split("  gate:")[0]
        self.assertEqual(problems(text), ["no gate job"])

    def test_a_gate_that_does_not_always_run_fails(self) -> None:
        for condition in ("", "    if: success()\n", "    if: ${{ always() }}\n", "    if: '!cancelled()'\n"):
            with self.subTest(condition=condition):
                text = GOOD.replace("    if: always()\n", condition)
                self.assertEqual(len(problems(text)), 1)
                self.assertIn("always()", problems(text)[0])

    def test_needs_in_another_form_is_refused_rather_than_read(self) -> None:
        block = "needs:\n      - check\n      - dist\n      - dist-run"
        for needs in (block, "needs: check"):
            with self.subTest(needs=needs):
                text = GOOD.replace("needs: [check, dist, dist-run]", needs)
                self.assertEqual(len(problems(text)), 1)
                self.assertIn("one-line list", problems(text)[0])

    def test_a_gate_without_needs_fails(self) -> None:
        text = GOOD.replace("    needs: [check, dist, dist-run]\n", "")
        self.assertEqual(problems(text), ["the gate needs no jobs"])

    def test_text_with_no_jobs_fails(self) -> None:
        self.assertEqual(problems("name: CI\n"), ["no jobs: key"])
        self.assertEqual(problems("name: CI\njobs:\n"), ["no jobs under jobs:"])


if __name__ == "__main__":
    unittest.main()
