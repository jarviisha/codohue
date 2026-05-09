#!/usr/bin/env pwsh
# Git-specific common functions for the git extension.
# Extracted from scripts/powershell/common.ps1 — contains only git-specific
# branch validation and detection logic.

function Test-HasGit {
    param([string]$RepoRoot = (Get-Location))
    try {
        if (-not (Test-Path (Join-Path $RepoRoot '.git'))) { return $false }
        if (-not (Get-Command git -ErrorAction SilentlyContinue)) { return $false }
        git -C $RepoRoot rev-parse --is-inside-work-tree 2>$null | Out-Null
        return ($LASTEXITCODE -eq 0)
    } catch {
        return $false
    }
}

function Get-SpecKitEffectiveBranchName {
    param([string]$Branch)
    if ($Branch -match '^([^/]+)/([^/]+)$') {
        return $Matches[2]
    }
    return $Branch
}

function Test-FeatureBranch {
    param(
        [string]$Branch,
        [bool]$HasGit = $true
    )

    # For non-git repos, we can't enforce branch naming but still provide output
    if (-not $HasGit) {
        Write-Warning "[specify] Warning: Git repository not detected; skipped branch validation"
        return $true
    }

    $pattern = '^(feat|fix|refactor|test|docs|ci|chore)/(api|cron|ingest|compute|recommend|nsconfig|auth|idmap|qdrant|redis|postgres|metrics|e2e|docs|ci)-[a-z0-9][a-z0-9-]*$'
    if ($Branch -match $pattern) { return $true }

    [Console]::Error.WriteLine("ERROR: Not on a feature branch. Current branch: $Branch")
    [Console]::Error.WriteLine("Feature branches should be named like: feat/api-feature-name or fix/recommend-bug-name")
    return $false
}
