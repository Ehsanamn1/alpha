# انتشار Alpha روی مخزن GitHub

مخزن مقصد: https://github.com/Ehsanamn1/alpha

## انتشار خودکار از Windows

1. Git، GitHub CLI (`gh`) و Go را نصب کن.
2. در PowerShell یک‌بار اجرا کن: `gh auth login` و با حساب `Ehsanamn1` وارد شو.
3. سورس ZIP را در یک پوشهٔ تازه استخراج کن.
4. در همان پوشه اجرا کن:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\Publish-Alpha-To-GitHub.ps1
```

اسکریپت پیش از push هویت حساب را بررسی می‌کند. فقط در صورتی اجازهٔ force-with-lease دارد که شاخهٔ `main` در مخزن مقصد هیچ فایلی نداشته باشد؛ اگر فایل دیگری روی آن باشد، متوقف می‌شود و چیزی را overwrite نمی‌کند. سپس سورس را push می‌کند و tag `v0.3.0-alpha` را برای اجرای pipeline انتشار می‌فرستد.

## انتشار از Git Bash / Linux / macOS

از ریشهٔ سورس اجرا کن:

```bash
gh auth login
./scripts/publish-github.sh
```

همان بررسی هویت، مخزن مقصد و خالی‌بودن tree شاخهٔ `main` اجرا می‌شود.

## پس از اجرا

- CI باید `go test -race ./...`, `go vet ./...`, قالب‌بندی و build را بگذراند.
- workflow انتشار، بسته‌های Windows x64، Linux x64/ARM64 و macOS Intel/Apple Silicon را می‌سازد.
- سایت استاتیک `site/` از طریق GitHub Pages منتشر می‌شود، اگر Pages در تنظیمات مخزن روی GitHub Actions فعال باشد.

بررسی نتایج:

- Actions: https://github.com/Ehsanamn1/alpha/actions
- Releases: https://github.com/Ehsanamn1/alpha/releases
- Website: https://ehsanamn1.github.io/alpha/

وجود tag به‌تنهایی به معنای انتشار موفق نیست؛ قبل از اشتراک لینک دانلود، باید workflow سبز شده و فایل‌ها داخل Releases قابل دریافت باشند.
