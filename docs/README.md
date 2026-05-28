# EliteGuard Documentation

| Document | Description |
|----------|-------------|
| [HTTP-gRPC-Gateway-Setup-Guide.md](./HTTP-gRPC-Gateway-Setup-Guide.md) | Full implementation guide (Markdown source) |
| **[EliteGuard-HTTP-gRPC-Gateway-Complete-Guide.pdf](./EliteGuard-HTTP-gRPC-Gateway-Complete-Guide.pdf)** | **Styled PDF — download & share** (all code + instructions) |

## Regenerate the PDF

Requires [Node.js](https://nodejs.org/) (v18+):

```powershell
cd "c:\Users\abdum\OneDrive\Desktop\New folder\Coding\CoreGuard Gateway\docs"
npm install puppeteer --no-save
node build_http_grpc_gateway_pdf.mjs
```

Output: `docs/EliteGuard-HTTP-gRPC-Gateway-Complete-Guide.pdf`

Alternative (Python + ReportLab, if Python is installed):

```powershell
pip install reportlab
python docs/build_http_grpc_gateway_pdf.py
```
