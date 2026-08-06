class ServeThem < Formula
  desc "Local multi-application HTTPS runtime"
  homepage "https://github.com/tankibaj/server-them"
  version "0.1.2"
  url "https://github.com/tankibaj/server-them/releases/download/v0.1.2/serve-them_0.1.2_darwin_arm64.tar.gz"
  sha256 "04ad65f389fd77a67bc599661b4fe726a81ed098bb5a50ac598d19066a673f3a"

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
