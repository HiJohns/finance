import sys
import json
import matplotlib.pyplot as plt

def main():
    try:
        data = json.loads(sys.stdin.read())
        names = list(data['corrs'].keys())
        values = [v[0] for v in data['corrs'].values()]

        plt.figure(figsize=(10, 6))
        colors = ['red' if v < -0.6 else 'skyblue' for v in values]
        plt.bar(names, values, color=colors)
        plt.axhline(y=-0.7, color='red', linestyle='--', label='Danger Zone')
        plt.ylim(-1, 1)
        plt.title("Asset Correlation vs DXY (Latest 6 Months)")
        plt.grid(axis='y', linestyle='--', alpha=0.7)
        plt.savefig("audit_chart.png")
    except Exception as e:
        print(f"Plotting error: {e}", file=sys.stderr)

if __name__ == "__main__":
    main()