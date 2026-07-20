param(
    [string]$GoExe = "",
    [string]$Version = "0.1.0-r20"
)

$ErrorActionPreference = "Stop"
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$dist = [IO.Path]::GetFullPath((Join-Path $root "dist"))
$bundleName = "iptv-refresh-$Version-x86_64"
$stage = [IO.Path]::GetFullPath((Join-Path $dist $bundleName))

if (-not $stage.StartsWith($dist + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Unsafe staging path: $stage"
}

if (-not $GoExe) {
    $command = Get-Command go -ErrorAction SilentlyContinue
    if ($command) {
        $GoExe = $command.Source
    } else {
        $portable = Join-Path $env:TEMP "codex-go1.26.5-2\go\bin\go.exe"
        if (Test-Path -LiteralPath $portable) {
            $GoExe = $portable
        }
    }
}
if (-not $GoExe -or -not (Test-Path -LiteralPath $GoExe)) {
    throw "Go compiler not found; pass -GoExe C:\path\to\go.exe"
}

New-Item -ItemType Directory -Force -Path $dist | Out-Null
if (Test-Path -LiteralPath $stage) {
    Remove-Item -LiteralPath $stage -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $stage | Out-Null

$oldCgo = $env:CGO_ENABLED
$oldGoos = $env:GOOS
$oldGoarch = $env:GOARCH
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    & $GoExe build -buildvcs=false -trimpath -ldflags "-s -w -X main.version=$Version" `
        -o (Join-Path $stage "iptv-refresh") ./cmd/iptv-refresh
    if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }
} finally {
    $env:CGO_ENABLED = $oldCgo
    $env:GOOS = $oldGoos
    $env:GOARCH = $oldGoarch
}

Copy-Item -LiteralPath (Join-Path $root "openwrt\files\install-bundle.sh") -Destination (Join-Path $stage "install.sh")
Copy-Item -LiteralPath (Join-Path $root "openwrt\files\iptv-refresh.init") -Destination $stage
Copy-Item -LiteralPath (Join-Path $root "openwrt\files\iptv-refresh-nginx-config") -Destination $stage
Copy-Item -LiteralPath (Join-Path $root "openwrt\files\iptv-refresh-scheduler") -Destination $stage
Copy-Item -LiteralPath (Join-Path $root "openwrt\files\iptv-refresh.uci") -Destination $stage
Copy-Item -LiteralPath (Join-Path $root "openwrt\files\provider.env") -Destination $stage
Copy-Item -LiteralPath (Join-Path $root "LICENSE") -Destination $stage
Copy-Item -LiteralPath (Join-Path $root "THIRD_PARTY_NOTICES.md") -Destination $stage

$archive = Join-Path $dist "$bundleName.tar.gz"
if (Test-Path -LiteralPath $archive) {
    Remove-Item -LiteralPath $archive -Force
}
& tar.exe -czf $archive -C $dist $bundleName
if ($LASTEXITCODE -ne 0) { throw "tar failed with exit code $LASTEXITCODE" }

$hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $archive).Hash.ToLowerInvariant()
$checksum = "$hash  $([IO.Path]::GetFileName($archive))`n"
[IO.File]::WriteAllText("$archive.sha256", $checksum, [Text.UTF8Encoding]::new($false))

Write-Output $archive
Write-Output "SHA256 $hash"
