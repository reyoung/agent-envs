FROM ghcr.io/astral-sh/uv:0.9-python3.14-trixie as py-313-env

RUN mkdir -p /envs && \
    cd /envs && \
    uv venv -p 3.13 py-3.13-env && \
    . py-3.13-env/bin/activate && \
    uv pip install torch torchvision --index-url https://download.pytorch.org/whl/cpu && \
    tar -czf py-3.13-env.tar.gz py-3.13-env    

FROM alpine:latest
COPY --from=py-313-env /envs/py-3.13-env.tar.gz /envs/py-3.13-env.tar.gz