type Props = {
  large?: boolean;
  showWordmark?: boolean;
};

export function BrandMark({ large = false, showWordmark = true }: Props) {
  return (
    <div className={`brand-mark ${large ? "large" : ""}`}>
      <img
        className="brand-logo"
        src="/logo.png"
        alt="Nimbus"
        width={large ? 40 : 28}
        height={large ? 40 : 28}
      />
      {showWordmark && <span className="brand-name">Nimbus</span>}
    </div>
  );
}
