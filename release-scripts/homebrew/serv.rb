class Serv < Formula
  desc "Programming language for background services, schedulers, and APIs"
  homepage "https://github.com/vyuvaraj/Serv-lang"
  version "1.0.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/vyuvaraj/Serv-lang/releases/download/v#{version}/serv-darwin-arm64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    else
      url "https://github.com/vyuvaraj/Serv-lang/releases/download/v#{version}/serv-darwin-amd64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/vyuvaraj/Serv-lang/releases/download/v#{version}/serv-linux-arm64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    else
      url "https://github.com/vyuvaraj/Serv-lang/releases/download/v#{version}/serv-linux-amd64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    end
  end

  depends_on "go" => :build

  def install
    # Install binaries
    bin.install "serv"
    bin.install "serv-lsp"

    # Install runtime, stdlib, and module files alongside the binary
    # These are needed for compilation (serv.exe finds them relative to itself)
    libexec.install "runtime"
    libexec.install "stdlib"
    libexec.install "go.mod"
    libexec.install "go.sum"
    libexec.install "declarations" if File.directory?("declarations")

    # Create wrapper scripts that set SERV_HOME
    (bin/"serv").unlink
    (bin/"serv").write <<~EOS
      #!/bin/bash
      export SERV_HOME="#{libexec}"
      exec "#{libexec}/serv" "$@"
    EOS

    # Copy actual binary to libexec
    libexec.install "serv"
  end

  test do
    # Verify the compiler runs
    assert_match "Serv: A Programming Language", shell_output("#{bin}/serv 2>&1", 0)

    # Verify init works
    system bin/"serv", "init", "test-project"
    assert_predicate testpath/"test-project/main.srv", :exist?
  end
end
