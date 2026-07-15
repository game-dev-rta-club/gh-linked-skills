---
title: Automation specification
updated: 2026-07-13
status: implemented
---

# Automation

## AUTO-001 CI

Main push/PR。Ubuntu/macOSでtest、race、vet、build。Permissionは`contents: read`。Release tagでもtest/vet。

## AUTO-002 Scheduled live E2E

Manual + 月曜03:17 UTC。Canonical repoだけでowned branchを作る。

Built binary自身のDiscoveryと`install --all`で2つのfixtureとmanifestを作り、status、push、remote pull、conflictを検査する。Evidence記録後、owned branchの削除を確認する。
