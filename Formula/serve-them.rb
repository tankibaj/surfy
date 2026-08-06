class ServeThem < Formula
  desc "Local multi-application HTTPS runtime"
  homepage "https://github.com/tankibaj/server-them"
  version "0.1.0"
  url "https://github.com/tankibaj/server-them/releases/download/v0.1.0/serve-them_0.1.0_darwin_arm64.tar.gz"
  sha256 "30d7a3f52e2df5934280206d9dc3e6f799d2f109b8c3ed0b552cf257815ca581"

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
