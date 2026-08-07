class Surfy < Formula
  desc "Local multi-application HTTPS runtime"
  homepage "https://github.com/tankibaj/surfy"
  version "0.1.1"
  url "https://github.com/tankibaj/surfy/releases/download/v0.1.1/surfy_0.1.1_darwin_arm64.tar.gz"
  sha256 "b1cced4fe6a2f9401fe0cdd0b491d5666479ff9aa6ce86ff1365d80108a2fa71"

  depends_on :macos
  depends_on arch: :arm64

  def install
    bin.install "surfy"
  end

  def caveats
    <<~EOS
      Complete the one-time HTTPS edge and Keychain setup with:
        surfy install
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/surfy version")
  end
end
