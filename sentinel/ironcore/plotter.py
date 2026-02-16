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
        
        vix_dxy_corr = data.get('vix_dxy_corr', 0)
        
        x = np.arange(len(names))
        width = 0.35
        
        fig, ax = plt.subplots(figsize=(12, 7))
        
        ax.bar(x - width/2, values6m, width, label='6mo Baseline', color='#87CEEB')
        
        colors30 = []
        for i, v30 in enumerate(values30):
            v6m = values6m[i]
            if v30 < -0.6 or v30 < (v6m - 0.2):
                colors30.append('#FF4500')
            else:
                colors30.append('#4682B4')
        
        ax.bar(x + width/2, values30, width, label='30d Current', color=colors30)
        
        ax.axhline(y=-0.7, color='red', linestyle='--', linewidth=1.5)
        
        ax.set_ylabel('Correlation')
        ax.set_title('IronCore: Asset Correlation Trend Audit')
        ax.set_xticks(x)
        ax.set_xticklabels(names)
        ax.legend()
        ax.set_ylim(-1, 1)
        ax.grid(axis='y', linestyle='--', alpha=0.7)
        
        for i, v in enumerate(values6m):
            ax.annotate(f'{v:.3f}', 
                       xy=(x[i] - width/2, v),
                       xytext=(0, -10 if v < 0 else 3),
                       textcoords='offset points',
                       ha='center', va='top' if v < 0 else 'bottom', fontsize=8)
        
        for i, v in enumerate(values30):
            if v != 0:
                ax.annotate(f'{v:.3f}', 
                           xy=(x[i] + width/2, v),
                           xytext=(0, -10 if v < 0 else 3),
                           textcoords='offset points',
                           ha='center', va='top' if v < 0 else 'bottom', fontsize=8)
        
        vix_color = 'red' if vix_dxy_corr > 0.5 else 'green'
        props = dict(boxstyle='round', facecolor='wheat', alpha=0.8)
        ax.text(0.02, 0.98, f'VIX-DXY Corr: {vix_dxy_corr:.3f}', 
                transform=ax.transAxes, fontsize=10, verticalalignment='top',
                bbox=props, color=vix_color, fontweight='bold')
        
        if vix_dxy_corr > 0.5:
            ax.text(0.02, 0.93, '⚠️ Liquidity Black Hole', 
                    transform=ax.transAxes, fontsize=9, verticalalignment='top',
                    color='red', fontweight='bold')
        
        plt.tight_layout()
        plt.savefig("audit_chart.png")
    except Exception as e:
        print(f"Plotting error: {e}", file=sys.stderr)

if __name__ == "__main__":
    main()