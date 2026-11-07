$ErrorActionPreference = 'Stop'

$repo = Split-Path -Parent $PSScriptRoot
$buildDir = (New-Item -ItemType Directory -Path (Join-Path $repo 'build') -Force).FullName
$stage = (New-Item -ItemType Directory -Path (Join-Path $buildDir "windows-demo-$([guid]::NewGuid())")).FullName
$exe = Join-Path $stage 'fin.exe'

Push-Location -LiteralPath $repo
$savedGOOS = $env:GOOS
$savedGOARCH = $env:GOARCH
$savedGOAMD64 = $env:GOAMD64
$savedCGO = $env:CGO_ENABLED
try {
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $env:GOAMD64 = 'v1'
    $env:CGO_ENABLED = '0'
    go build -trimpath -o $exe ./cmd/fin
    if ($LASTEXITCODE -ne 0) { throw "go build failed ($LASTEXITCODE)." }

    $version = & $exe -version
    if ($LASTEXITCODE -ne 0 -or $version -notmatch '^fin (v\d+\.\d+\.\d+)$') {
        throw "Unexpected executable version '$version'."
    }
    $archive = Join-Path $buildDir "finlang-$($Matches[1])-windows-amd64.zip"

    Copy-Item -LiteralPath 'scripts/windows-demo.cmd' -Destination (Join-Path $stage 'run-demo.cmd')
    Copy-Item -LiteralPath 'examples/interest_rule_v1.fin' -Destination $stage
    Copy-Item -LiteralPath 'testdata/inputs/interest.json' -Destination $stage
    Copy-Item -LiteralPath 'contracts/interest_rule_v1.json' -Destination (Join-Path $stage 'interest_rule_v1.contract.json')
    Copy-Item -LiteralPath 'testdata/expected/interest_rule_v1.json' -Destination (Join-Path $stage 'expected.json')
    Copy-Item -LiteralPath 'docs/windows-demo.md' -Destination (Join-Path $stage 'README.md')
    $goRoot = go env GOROOT
    if ($LASTEXITCODE -ne 0) { throw "go env GOROOT failed ($LASTEXITCODE)." }
    Copy-Item -LiteralPath (Join-Path $goRoot 'LICENSE') -Destination (Join-Path $stage 'Go-LICENSE.txt')

    $files = Get-ChildItem -LiteralPath $stage -File | Sort-Object Name | ForEach-Object FullName
    Compress-Archive -LiteralPath $files -DestinationPath $archive -Force
    Write-Output $archive
} finally {
    $env:GOOS = $savedGOOS
    $env:GOARCH = $savedGOARCH
    $env:GOAMD64 = $savedGOAMD64
    $env:CGO_ENABLED = $savedCGO
    Pop-Location

    $resolvedStage = (Resolve-Path -LiteralPath $stage).ProviderPath
    if ([System.IO.Path]::GetDirectoryName($resolvedStage) -eq $buildDir) {
        Remove-Item -LiteralPath $resolvedStage -Recurse -Force
    }
}
