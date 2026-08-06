class ServeThem < Formula
  desc "Local multi-application HTTPS runtime"
  homepage "https://github.com/tankibaj/server-them"
  version "0.1.1"
  url "https://github.com/tankibaj/server-them/releases/download/v0.1.1/serve-them_0.1.1_darwin_arm64.tar.gz"
  sha256 "090562a0380be3417877e862d826d5fa460b79ac984490fe89f1f52e3245184e"

  depends_on :macos
  depends_on arch: :arm64

  def install
    bin.install "serve-them"
  end

  def caveats
    <<~EOS
      Complete the one-time HTTPS edge and Keychain setup with:
        serve-them install
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/serve-them version")
  end
end
