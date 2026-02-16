import sys
import json
import matplotlib.pyplot as plt
import numpy as np

def main():
    try:
        data = json.loads(sys.stdin.read())
        
        if 'corrs6m' not in data:
            print("Missing corrs6m data", file=sys.stderr)
            return
            
        names = list(data['corrs6m'].keys())
        values6m = [v[0] for v in data['corrs6m'].values()]
        
        values30 = []
        for name in names:
            if name in data.get('corrs30', {}):
                values30.append(data['corrs30'][name][0])
            else:
                values30.append(0)
        
        x = np.arange(len(names))
        width = 0.35
        
        fig, ax = plt.subplots(figsize=(12, 6))
        
        bars6m = ax.bar(x - width/2, values6m, width, label='6M Baseline', color='lightsteelblue')
        
        colors30 = []
        deltas = []
        for i, v30 in enumerate(values30):
            delta = v30 - values6m[i]
            deltas.append(delta)
            if delta < -0.2:
                colors30.append('red')
            else:
                colors30.append('steelblue')
        
        bars30 = ax.bar(x + width/2, values30, width, label='30D Current', color=colors30)
        
        ax.axhline(y=-0.7, color='red', linestyle='--', linewidth=1.5, label='Danger Zone')
        
        ax.set_ylabel('Correlation')
        ax.set_title('IronCore: Multi-Timeframe Correlation Audit')
        ax.set_xticks(x)
        ax.set_xticklabels(names)
        ax.legend()
        ax.set_ylim(-1, 1)
        ax.grid(axis='y', linestyle='--', alpha=0.7)
        
        for i, delta in enumerate(deltas):
            if values30[i] != 0:
                ax.annotate(f'Δ:{delta:.2f}', 
                           xy=(x[i] + width/2, values30[i]),
                           xytext=(0, 3),
                           textcoords='offset points',
                           ha='center', va='bottom', fontsize=8)
        
        plt.tight_layout()
        plt.savefig("audit_chart.png")
    except Exception as e:
        print(f"Plotting error: {e}", file=sys.stderr)

if __name__ == "__main__":
    main()